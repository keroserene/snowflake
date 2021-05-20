/*
Broker acts as the HTTP signaling channel.
It matches clients and snowflake proxies by passing corresponding
SessionDescriptions in order to negotiate a WebRTC connection.
*/
package main

import (
	"container/heap"
	"crypto/tls"
	"errors"
	"flag"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"git.torproject.org/pluggable-transports/snowflake.git/common/messages"
	"git.torproject.org/pluggable-transports/snowflake.git/common/safelog"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/crypto/acme/autocert"
)

const (
	readLimit = 100000 // Maximum number of bytes to be read from an HTTP request
)

type BrokerContext struct {
	snowflakes           *SnowflakeHeap
	restrictedSnowflakes *SnowflakeHeap
	// Maps keeping track of snowflakeIDs required to match SDP answers from
	// the second http POST. Restricted snowflakes can only be matched up with
	// clients behind an unrestricted NAT.
	idToSnowflake map[string]*Snowflake
	// Synchronization for the snowflake map and heap
	snowflakeLock sync.Mutex
	proxyPolls    chan *ProxyPoll
	metrics       *Metrics
}

func NewBrokerContext(metricsLogger *log.Logger) *BrokerContext {
	snowflakes := new(SnowflakeHeap)
	heap.Init(snowflakes)
	rSnowflakes := new(SnowflakeHeap)
	heap.Init(rSnowflakes)
	metrics, err := NewMetrics(metricsLogger)

	if err != nil {
		panic(err.Error())
	}

	if metrics == nil {
		panic("Failed to create metrics")
	}

	return &BrokerContext{
		snowflakes:           snowflakes,
		restrictedSnowflakes: rSnowflakes,
		idToSnowflake:        make(map[string]*Snowflake),
		proxyPolls:           make(chan *ProxyPoll),
		metrics:              metrics,
	}
}

// Implements the http.Handler interface
type SnowflakeHandler struct {
	*IPC
	handle func(*IPC, http.ResponseWriter, *http.Request)
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
	sh.handle(sh.IPC, w, r)
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
	proxyType    string
	natType      string
	clients      int
	offerChannel chan *ClientOffer
}

// Registers a Snowflake and waits for some Client to send an offer,
// as part of the polling logic of the proxy handler.
func (ctx *BrokerContext) RequestOffer(id string, proxyType string, natType string, clients int) *ClientOffer {
	request := new(ProxyPoll)
	request.id = id
	request.proxyType = proxyType
	request.natType = natType
	request.clients = clients
	request.offerChannel = make(chan *ClientOffer)
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
		snowflake := ctx.AddSnowflake(request.id, request.proxyType, request.natType, request.clients)
		// Wait for a client to avail an offer to the snowflake.
		go func(request *ProxyPoll) {
			select {
			case offer := <-snowflake.offerChannel:
				request.offerChannel <- offer
			case <-time.After(time.Second * ProxyTimeout):
				// This snowflake is no longer available to serve clients.
				ctx.snowflakeLock.Lock()
				defer ctx.snowflakeLock.Unlock()
				if snowflake.index != -1 {
					if request.natType == NATUnrestricted {
						heap.Remove(ctx.snowflakes, snowflake.index)
					} else {
						heap.Remove(ctx.restrictedSnowflakes, snowflake.index)
					}
					ctx.metrics.promMetrics.AvailableProxies.With(prometheus.Labels{"nat": request.natType, "type": request.proxyType}).Dec()
					delete(ctx.idToSnowflake, snowflake.id)
					close(request.offerChannel)
				}
			}
		}(request)
	}
}

// Create and add a Snowflake to the heap.
// Required to keep track of proxies between providing them
// with an offer and awaiting their second POST with an answer.
func (ctx *BrokerContext) AddSnowflake(id string, proxyType string, natType string, clients int) *Snowflake {
	snowflake := new(Snowflake)
	snowflake.id = id
	snowflake.clients = clients
	snowflake.proxyType = proxyType
	snowflake.natType = natType
	snowflake.offerChannel = make(chan *ClientOffer)
	snowflake.answerChannel = make(chan string)
	ctx.snowflakeLock.Lock()
	if natType == NATUnrestricted {
		heap.Push(ctx.snowflakes, snowflake)
	} else {
		heap.Push(ctx.restrictedSnowflakes, snowflake)
	}
	ctx.metrics.promMetrics.AvailableProxies.With(prometheus.Labels{"nat": natType, "type": proxyType}).Inc()
	ctx.snowflakeLock.Unlock()
	ctx.idToSnowflake[id] = snowflake
	return snowflake
}

/*
For snowflake proxies to request a client from the Broker.
*/
func proxyPolls(i *IPC, w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(http.MaxBytesReader(w, r.Body, readLimit))
	if err != nil {
		log.Println("Invalid data.")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	arg := messages.Arg{
		Body:       body,
		RemoteAddr: r.RemoteAddr,
		NatType:    "",
	}

	var response []byte
	err = i.ProxyPolls(arg, &response)
	switch {
	case err == nil:
	case errors.Is(err, messages.ErrBadRequest):
		w.WriteHeader(http.StatusBadRequest)
		return
	case errors.Is(err, messages.ErrInternal):
		fallthrough
	default:
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if _, err := w.Write(response); err != nil {
		log.Printf("proxyPolls unable to write offer with error: %v", err)
	}
}

// Client offer contains an SDP and the NAT type of the client
type ClientOffer struct {
	natType string
	sdp     []byte
}

/*
Expects a WebRTC SDP offer in the Request to give to an assigned
snowflake proxy, which responds with the SDP answer to be sent in
the HTTP response back to the client.
*/
func clientOffers(i *IPC, w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(http.MaxBytesReader(w, r.Body, readLimit))
	if err != nil {
		log.Printf("Error reading client request: %s", err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	arg := messages.Arg{
		Body:       body,
		RemoteAddr: "",
		NatType:    r.Header.Get("Snowflake-NAT-Type"),
	}

	var response []byte
	err = i.ClientOffers(arg, &response)
	switch {
	case err == nil:
	case errors.Is(err, messages.ErrUnavailable):
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	case errors.Is(err, messages.ErrTimeout):
		w.WriteHeader(http.StatusGatewayTimeout)
		return
	default:
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if _, err := w.Write(response); err != nil {
		log.Printf("clientOffers unable to write answer with error: %v", err)
	}
}

/*
Expects snowflake proxes which have previously successfully received
an offer from proxyHandler to respond with an answer in an HTTP POST,
which the broker will pass back to the original client.
*/
func proxyAnswers(i *IPC, w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(http.MaxBytesReader(w, r.Body, readLimit))
	if err != nil {
		log.Println("Invalid data.")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	arg := messages.Arg{
		Body:       body,
		RemoteAddr: "",
		NatType:    "",
	}

	var response []byte
	err = i.ProxyAnswers(arg, &response)
	switch {
	case err == nil:
	case errors.Is(err, messages.ErrBadRequest):
		w.WriteHeader(http.StatusBadRequest)
		return
	case errors.Is(err, messages.ErrInternal):
		fallthrough
	default:
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if _, err := w.Write(response); err != nil {
		log.Printf("proxyAnswers unable to write answer response with error: %v", err)
	}
}

func debugHandler(i *IPC, w http.ResponseWriter, r *http.Request) {
	var response string

	err := i.Debug(new(interface{}), &response)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if _, err := w.Write([]byte(response)); err != nil {
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
	var unsafeLogging bool

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
	flag.BoolVar(&unsafeLogging, "unsafe-logging", false, "prevent logs from being scrubbed")
	flag.Parse()

	var err error
	var metricsFile io.Writer
	var logOutput io.Writer = os.Stderr
	if unsafeLogging {
		log.SetOutput(logOutput)
	} else {
		// We want to send the log output through our scrubber first
		log.SetOutput(&safelog.LogScrubber{Output: logOutput})
	}

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

	i := &IPC{ctx}

	http.HandleFunc("/robots.txt", robotsTxtHandler)

	http.Handle("/proxy", SnowflakeHandler{i, proxyPolls})
	http.Handle("/client", SnowflakeHandler{i, clientOffers})
	http.Handle("/answer", SnowflakeHandler{i, proxyAnswers})
	http.Handle("/debug", SnowflakeHandler{i, debugHandler})
	http.Handle("/metrics", MetricsHandler{metricsFilename, metricsHandler})
	http.Handle("/prometheus", promhttp.HandlerFor(ctx.metrics.promMetrics.registry, promhttp.HandlerOpts{}))

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
