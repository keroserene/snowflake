/*
Broker acts as the HTTP signaling channel.
It matches clients and snowflake proxies by passing corresponding
SessionDescriptions in order to negotiate a WebRTC connection.
*/
package main

import (
	"container/heap"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"git.torproject.org/pluggable-transports/snowflake.git/common/messages"
	"git.torproject.org/pluggable-transports/snowflake.git/common/safelog"
	"golang.org/x/crypto/acme/autocert"
)

const (
	ClientTimeout = 10
	ProxyTimeout  = 10
	readLimit     = 100000 //Maximum number of bytes to be read from an HTTP request
)

type BrokerContext struct {
	snowflakes *SnowflakeHeap
	// Map keeping track of snowflakeIDs required to match SDP answers from
	// the second http POST.
	idToSnowflake map[string]*Snowflake
	proxyPolls    chan *ProxyPoll
	metrics       *Metrics
}

func NewBrokerContext(metricsLogger *log.Logger) *BrokerContext {
	snowflakes := new(SnowflakeHeap)
	heap.Init(snowflakes)
	metrics, err := NewMetrics(metricsLogger)

	if err != nil {
		panic(err.Error())
	}

	if metrics == nil {
		panic("Failed to create metrics")
	}

	return &BrokerContext{
		snowflakes:    snowflakes,
		idToSnowflake: make(map[string]*Snowflake),
		proxyPolls:    make(chan *ProxyPoll),
		metrics:       metrics,
	}
}

// Implements the http.Handler interface
type SnowflakeHandler struct {
	*BrokerContext
	handle func(*BrokerContext, http.ResponseWriter, *http.Request)
}

// Implements the http.Handler interface
type MetricsHandler struct {
	logFilename string
	handle      func(string, http.ResponseWriter, *http.Request)
}

func (sh SnowflakeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Origin, X-Session-ID")
	// Return early if it's CORS preflight.
	if "OPTIONS" == r.Method {
		return
	}
	sh.handle(sh.BrokerContext, w, r)
}

func (mh MetricsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Origin, X-Session-ID")
	// Return early if it's CORS preflight.
	if "OPTIONS" == r.Method {
		return
	}
	mh.handle(mh.logFilename, w, r)
}

// Proxies may poll for client offers concurrently.
type ProxyPoll struct {
	id           string
	offerChannel chan []byte
}

// Registers a Snowflake and waits for some Client to send an offer,
// as part of the polling logic of the proxy handler.
func (ctx *BrokerContext) RequestOffer(id string) []byte {
	request := new(ProxyPoll)
	request.id = id
	request.offerChannel = make(chan []byte)
	ctx.proxyPolls <- request
	// Block until an offer is available, or timeout which sends a nil offer.
	offer := <-request.offerChannel
	return offer
}

// goroutine which matches clients to proxies and sends SDP offers along.
// Safely processes proxy requests, responding to them with either an available
// client offer or nil on timeout / none are available.
func (ctx *BrokerContext) Broker() {
	for request := range ctx.proxyPolls {
		snowflake := ctx.AddSnowflake(request.id)
		// Wait for a client to avail an offer to the snowflake.
		go func(request *ProxyPoll) {
			select {
			case offer := <-snowflake.offerChannel:
				request.offerChannel <- offer
			case <-time.After(time.Second * ProxyTimeout):
				// This snowflake is no longer available to serve clients.
				// TODO: Fix race using a delete channel
				heap.Remove(ctx.snowflakes, snowflake.index)
				delete(ctx.idToSnowflake, snowflake.id)
				request.offerChannel <- nil
			}
		}(request)
	}
}

// Create and add a Snowflake to the heap.
// Required to keep track of proxies between providing them
// with an offer and awaiting their second POST with an answer.
func (ctx *BrokerContext) AddSnowflake(id string) *Snowflake {
	snowflake := new(Snowflake)
	snowflake.id = id
	snowflake.clients = 0
	snowflake.offerChannel = make(chan []byte)
	snowflake.answerChannel = make(chan []byte)
	heap.Push(ctx.snowflakes, snowflake)
	ctx.idToSnowflake[id] = snowflake
	return snowflake
}

/*
For snowflake proxies to request a client from the Broker.
*/
func proxyPolls(ctx *BrokerContext, w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(http.MaxBytesReader(w, r.Body, readLimit))
	if err != nil {
		log.Println("Invalid data.")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	sid, err := messages.DecodePollRequest(body)
	if err != nil {
		log.Println("Invalid data.")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Log geoip stats
	remoteIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		log.Println("Error processing proxy IP: ", err.Error())
	} else {
		ctx.metrics.UpdateCountryStats(remoteIP)
	}

	// Wait for a client to avail an offer to the snowflake, or timeout if nil.
	offer := ctx.RequestOffer(sid)
	var b []byte
	if nil == offer {
		ctx.metrics.proxyIdleCount++

		b, err = messages.EncodePollResponse("", false)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Write(b)
		return
	}
	b, err = messages.EncodePollResponse(string(offer), true)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if _, err := w.Write(b); err != nil {
		log.Printf("proxyPolls unable to write offer with error: %v", err)
	}
}

/*
Expects a WebRTC SDP offer in the Request to give to an assigned
snowflake proxy, which responds with the SDP answer to be sent in
the HTTP response back to the client.
*/
func clientOffers(ctx *BrokerContext, w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	offer, err := ioutil.ReadAll(http.MaxBytesReader(w, r.Body, readLimit))
	if nil != err {
		log.Println("Invalid data.")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	// Immediately fail if there are no snowflakes available.
	if ctx.snowflakes.Len() <= 0 {
		ctx.metrics.clientDeniedCount++
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	// Otherwise, find the most available snowflake proxy, and pass the offer to it.
	// Delete must be deferred in order to correctly process answer request later.
	snowflake := heap.Pop(ctx.snowflakes).(*Snowflake)
	defer delete(ctx.idToSnowflake, snowflake.id)
	snowflake.offerChannel <- offer

	// Wait for the answer to be returned on the channel or timeout.
	select {
	case answer := <-snowflake.answerChannel:
		ctx.metrics.clientProxyMatchCount++
		if _, err := w.Write(answer); err != nil {
			log.Printf("unable to write answer with error: %v", err)
		}
		// Initial tracking of elapsed time.
		ctx.metrics.clientRoundtripEstimate = time.Since(startTime) /
			time.Millisecond
	case <-time.After(time.Second * ClientTimeout):
		log.Println("Client: Timed out.")
		w.WriteHeader(http.StatusGatewayTimeout)
		if _, err := w.Write([]byte("timed out waiting for answer!")); err != nil {
			log.Printf("unable to write timeout error, failed with error: %v", err)
		}
	}
}

/*
Expects snowflake proxes which have previously successfully received
an offer from proxyHandler to respond with an answer in an HTTP POST,
which the broker will pass back to the original client.
*/
func proxyAnswers(ctx *BrokerContext, w http.ResponseWriter, r *http.Request) {

	body, err := ioutil.ReadAll(http.MaxBytesReader(w, r.Body, readLimit))
	if nil != err || nil == body || len(body) <= 0 {
		log.Println("Invalid data.")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	answer, id, err := messages.DecodeAnswerRequest(body)
	if err != nil || answer == "" {
		log.Println("Invalid data.")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var success = true
	snowflake, ok := ctx.idToSnowflake[id]
	if !ok || nil == snowflake {
		// The snowflake took too long to respond with an answer, so its client
		// disappeared / the snowflake is no longer recognized by the Broker.
		success = false
	}
	b, err := messages.EncodeAnswerResponse(success)
	if err != nil {
		log.Printf("Error encoding answer: %s", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(b)

	if success {
		snowflake.answerChannel <- []byte(answer)
	}

}

func debugHandler(ctx *BrokerContext, w http.ResponseWriter, r *http.Request) {
	s := fmt.Sprintf("current snowflakes available: %d\n", ctx.snowflakes.Len())

	var browsers, standalones int
	for _, snowflake := range ctx.idToSnowflake {
		if len(snowflake.id) < 16 {
			browsers++
		} else {
			standalones++
		}
	}
	s += fmt.Sprintf("\tstandalone proxies: %d", standalones)
	s += fmt.Sprintf("\n\tbrowser proxies: %d", browsers)
	if _, err := w.Write([]byte(s)); err != nil {
		log.Printf("writing proxy information returned error: %v ", err)
	}
}

func robotsTxtHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if _, err := w.Write([]byte("User-agent: *\nDisallow: /\n")); err != nil {
		log.Printf("robotsTxtHandler unable to write, with this error: %v", err)
	}
}

func metricsHandler(metricsFilename string, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	if metricsFilename == "" {
		http.NotFound(w, r)
		return
	}
	metricsFile, err := os.OpenFile(metricsFilename, os.O_RDONLY, 0644)
	if err != nil {
		log.Println("Error opening metrics file for reading")
		http.NotFound(w, r)
		return
	}

	if _, err := io.Copy(w, metricsFile); err != nil {
		log.Printf("copying metricsFile returned error: %v", err)
	}
}

func main() {
	var acmeEmail string
	var acmeHostnamesCommas string
	var acmeCertCacheDir string
	var addr string
	var geoipDatabase string
	var geoip6Database string
	var disableTLS bool
	var certFilename, keyFilename string
	var disableGeoip bool
	var metricsFilename string

	flag.StringVar(&acmeEmail, "acme-email", "", "optional contact email for Let's Encrypt notifications")
	flag.StringVar(&acmeHostnamesCommas, "acme-hostnames", "", "comma-separated hostnames for TLS certificate")
	flag.StringVar(&certFilename, "cert", "", "TLS certificate file")
	flag.StringVar(&keyFilename, "key", "", "TLS private key file")
	flag.StringVar(&acmeCertCacheDir, "acme-cert-cache", "acme-cert-cache", "directory in which certificates should be cached")
	flag.StringVar(&addr, "addr", ":443", "address to listen on")
	flag.StringVar(&geoipDatabase, "geoipdb", "/usr/share/tor/geoip", "path to correctly formatted geoip database mapping IPv4 address ranges to country codes")
	flag.StringVar(&geoip6Database, "geoip6db", "/usr/share/tor/geoip6", "path to correctly formatted geoip database mapping IPv6 address ranges to country codes")
	flag.BoolVar(&disableTLS, "disable-tls", false, "don't use HTTPS")
	flag.BoolVar(&disableGeoip, "disable-geoip", false, "don't use geoip for stats collection")
	flag.StringVar(&metricsFilename, "metrics-log", "", "path to metrics logging output")
	flag.Parse()

	var err error
	var metricsFile io.Writer
	var logOutput io.Writer = os.Stderr
	//We want to send the log output through our scrubber first
	log.SetOutput(&safelog.LogScrubber{Output: logOutput})

	log.SetFlags(log.LstdFlags | log.LUTC)

	if metricsFilename != "" {
		metricsFile, err = os.OpenFile(metricsFilename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)

		if err != nil {
			log.Fatal(err.Error())
		}
	} else {
		metricsFile = os.Stdout
	}

	metricsLogger := log.New(metricsFile, "", 0)

	ctx := NewBrokerContext(metricsLogger)

	if !disableGeoip {
		err = ctx.metrics.LoadGeoipDatabases(geoipDatabase, geoip6Database)
		if err != nil {
			log.Fatal(err.Error())
		}
	}

	go ctx.Broker()

	http.HandleFunc("/robots.txt", robotsTxtHandler)

	http.Handle("/proxy", SnowflakeHandler{ctx, proxyPolls})
	http.Handle("/client", SnowflakeHandler{ctx, clientOffers})
	http.Handle("/answer", SnowflakeHandler{ctx, proxyAnswers})
	http.Handle("/debug", SnowflakeHandler{ctx, debugHandler})
	http.Handle("/metrics", MetricsHandler{metricsFilename, metricsHandler})

	server := http.Server{
		Addr: addr,
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGHUP)

	// go routine to handle a SIGHUP signal to allow the broker operator to send
	// a SIGHUP signal when the geoip database files are updated, without requiring
	// a restart of the broker
	go func() {
		for {
			signal := <-sigChan
			log.Printf("Received signal: %s. Reloading geoip databases.", signal)
			if err = ctx.metrics.LoadGeoipDatabases(geoipDatabase, geoip6Database); err != nil {
				log.Fatalf("reload of Geo IP databases on signal %s returned error: %v", signal, err)
			}
		}
	}()

	// Handle the various ways of setting up TLS. The legal configurations
	// are:
	//   --acme-hostnames (with optional --acme-email and/or --acme-cert-cache)
	//   --cert and --key together
	//   --disable-tls
	// The outputs of this block of code are the disableTLS,
	// needHTTP01Listener, certManager, and getCertificate variables.
	if acmeHostnamesCommas != "" {
		acmeHostnames := strings.Split(acmeHostnamesCommas, ",")
		log.Printf("ACME hostnames: %q", acmeHostnames)

		var cache autocert.Cache
		if err = os.MkdirAll(acmeCertCacheDir, 0700); err != nil {
			log.Printf("Warning: Couldn't create cache directory %q (reason: %s) so we're *not* using our certificate cache.", acmeCertCacheDir, err)
		} else {
			cache = autocert.DirCache(acmeCertCacheDir)
		}

		certManager := autocert.Manager{
			Cache:      cache,
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(acmeHostnames...),
			Email:      acmeEmail,
		}
		go func() {
			log.Printf("Starting HTTP-01 listener")
			log.Fatal(http.ListenAndServe(":80", certManager.HTTPHandler(nil)))
		}()

		server.TLSConfig = &tls.Config{GetCertificate: certManager.GetCertificate}
		err = server.ListenAndServeTLS("", "")
	} else if certFilename != "" && keyFilename != "" {
		if acmeEmail != "" || acmeHostnamesCommas != "" {
			log.Fatalf("The --cert and --key options are not allowed with --acme-email or --acme-hostnames.")
		}
		err = server.ListenAndServeTLS(certFilename, keyFilename)
	} else if disableTLS {
		err = server.ListenAndServe()
	} else {
		log.Fatal("the --acme-hostnames, --cert and --key, or --disable-tls option is required")
	}

	if err != nil {
		log.Fatal(err)
	}
}
