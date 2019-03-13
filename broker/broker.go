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
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/acme/autocert"
)

const (
	ClientTimeout = 10
	ProxyTimeout  = 10
)

type BrokerContext struct {
	snowflakes *SnowflakeHeap
	// Map keeping track of snowflakeIDs required to match SDP answers from
	// the second http POST.
	idToSnowflake map[string]*Snowflake
	proxyPolls    chan *ProxyPoll
	metrics       *Metrics
}

func NewBrokerContext() *BrokerContext {
	snowflakes := new(SnowflakeHeap)
	heap.Init(snowflakes)
	metrics, err := NewMetrics()

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

func (sh SnowflakeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Origin, X-Session-ID")
	// Return early if it's CORS preflight.
	if "OPTIONS" == r.Method {
		return
	}
	sh.handle(sh.BrokerContext, w, r)
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
				log.Println("Passing client offer to snowflake proxy.")
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
	id := r.Header.Get("X-Session-ID")
	body, err := ioutil.ReadAll(r.Body)
	if nil != err {
		log.Println("Invalid data.")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if string(body) != id {
		log.Println("Mismatched IDs!")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	log.Println("Received snowflake: ", id)
	// Wait for a client to avail an offer to the snowflake, or timeout if nil.
	offer := ctx.RequestOffer(id)
	if nil == offer {
		log.Println("Proxy " + id + " did not receive a Client offer.")
		w.WriteHeader(http.StatusGatewayTimeout)
		return
	}
	log.Println("Passing client offer to snowflake.")
	w.Write(offer)
}

/*
Expects a WebRTC SDP offer in the Request to give to an assigned
snowflake proxy, which responds with the SDP answer to be sent in
the HTTP response back to the client.
*/
func clientOffers(ctx *BrokerContext, w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	offer, err := ioutil.ReadAll(r.Body)
	if nil != err {
		log.Println("Invalid data.")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	// Immediately fail if there are no snowflakes available.
	if ctx.snowflakes.Len() <= 0 {
		log.Println("Client: No snowflake proxies available.")
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
		log.Println("Client: Retrieving answer")
		w.Write(answer)
		// Initial tracking of elapsed time.
		ctx.metrics.clientRoundtripEstimate = time.Since(startTime) /
			time.Millisecond
	case <-time.After(time.Second * ClientTimeout):
		log.Println("Client: Timed out.")
		w.WriteHeader(http.StatusGatewayTimeout)
		w.Write([]byte("timed out waiting for answer!"))
	}
}

/*
Expects snowflake proxes which have previously successfully received
an offer from proxyHandler to respond with an answer in an HTTP POST,
which the broker will pass back to the original client.
*/
func proxyAnswers(ctx *BrokerContext, w http.ResponseWriter, r *http.Request) {
	id := r.Header.Get("X-Session-ID")
	snowflake, ok := ctx.idToSnowflake[id]
	if !ok || nil == snowflake {
		// The snowflake took too long to respond with an answer, so its client
		// disappeared / the snowflake is no longer recognized by the Broker.
		w.WriteHeader(http.StatusGone)
		return
	}
	body, err := ioutil.ReadAll(r.Body)
	if nil != err || nil == body || len(body) <= 0 {
		log.Println("Invalid data.")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Get proxy country stats
	remoteIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		log.Println("Error processing proxy IP: ", err.Error())
	} else {

		ctx.metrics.UpdateCountryStats(remoteIP)
	}

	log.Println("Received answer: ", body)
	snowflake.answerChannel <- body
}

func debugHandler(ctx *BrokerContext, w http.ResponseWriter, r *http.Request) {
	s := fmt.Sprintf("current snowflakes available: %d\n", ctx.snowflakes.Len())
	for _, snowflake := range ctx.idToSnowflake {
		s += fmt.Sprintf("\nsnowflake %d: %s", snowflake.index, snowflake.id)
	}
	s += fmt.Sprintf("\n\nroundtrip avg: %d", ctx.metrics.clientRoundtripEstimate)
	s += fmt.Sprintf("\n\nsnowflake country stats: %s", ctx.metrics.countryStats.Display())
	w.Write([]byte(s))
}

func robotsTxtHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte("User-agent: *\nDisallow:\n"))
}

func main() {
	var acmeEmail string
	var acmeHostnamesCommas string
	var addr string
	var geoipDatabase string
	var geoip6Database string
	var disableTLS bool
	var disableGeoip bool

	flag.StringVar(&acmeEmail, "acme-email", "", "optional contact email for Let's Encrypt notifications")
	flag.StringVar(&acmeHostnamesCommas, "acme-hostnames", "", "comma-separated hostnames for TLS certificate")
	flag.StringVar(&addr, "addr", ":443", "address to listen on")
	flag.StringVar(&geoipDatabase, "geoipdb", "/usr/share/tor/geoip", "path to correctly formatted geoip database mapping IPv4 address ranges to country codes")
	flag.StringVar(&geoip6Database, "geoip6db", "/usr/share/tor/geoip6", "path to correctly formatted geoip database mapping IPv6 address ranges to country codes")
	flag.BoolVar(&disableTLS, "disable-tls", false, "don't use HTTPS")
	flag.BoolVar(&disableGeoip, "disable-geoip", false, "don't use geoip for stats collection")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.LUTC)

	ctx := NewBrokerContext()

	if !disableGeoip {
		err := ctx.metrics.LoadGeoipDatabases(geoipDatabase, geoip6Database)
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

	var err error
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
			log.Println("Received signal:", signal, ". Reloading geoip databases.")
			ctx.metrics.LoadGeoipDatabases(geoipDatabase, geoip6Database)
		}
	}()

	if acmeHostnamesCommas != "" {
		acmeHostnames := strings.Split(acmeHostnamesCommas, ",")
		log.Printf("ACME hostnames: %q", acmeHostnames)

		certManager := autocert.Manager{
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
	} else if disableTLS {
		err = server.ListenAndServe()
	} else {
		log.Fatal("the --acme-hostnames or --disable-tls option is required")
	}

	if err != nil {
		log.Fatal(err)
	}
}
