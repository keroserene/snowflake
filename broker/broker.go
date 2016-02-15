/*
Broker acts as the HTTP signaling channel.
It matches clients and snowflake proxies by passing corresponding
SessionDescriptions in order to negotiate a WebRTC connection.

TODO(serene): This code is currently the absolute minimum required to
cause a successful negotiation.
It's otherwise very unsafe and problematic, and needs quite some work...
*/
package snowflake_broker

import (
	"container/heap"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"time"
)

const (
	ClientTimeout = 10
	ProxyTimeout  = 10
)

type BrokerContext struct {
	snowflakes *SnowflakeHeap
	// Map keeping track of snowflakeIDs required to match SDP answers from
	// the second http POST.
	snowflakeMap map[string]*Snowflake
	createChan   chan *ProxyRequest
}

func NewBrokerContext() *BrokerContext {
	snowflakes := new(SnowflakeHeap)
	heap.Init(snowflakes)
	return &BrokerContext{
		snowflakes:   snowflakes,
		snowflakeMap: make(map[string]*Snowflake),
		createChan:   make(chan *ProxyRequest),
	}
}

type SnowflakeHandler struct {
	*BrokerContext
	h func(*BrokerContext, http.ResponseWriter, *http.Request)
}

func (sh SnowflakeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sh.h(sh.BrokerContext, w, r)
}

type ProxyRequest struct {
	id        string
	offerChan chan []byte
}

// Create and add a Snowflake to the heap.
func (sc *BrokerContext) AddSnowflake(id string) *Snowflake {
	snowflake := new(Snowflake)
	snowflake.id = id
	snowflake.clients = 0
	snowflake.offerChannel = make(chan []byte)
	snowflake.answerChannel = make(chan []byte)
	heap.Push(sc.snowflakes, snowflake)
	sc.snowflakeMap[id] = snowflake
	return snowflake
}

// Match proxies to clients.
// func (ctx *BrokerContext) Broker(proxies <-chan *ProxyRequest) {
func (ctx *BrokerContext) Broker() {
	// for p := range proxies {
	for p := range ctx.createChan {
		snowflake := ctx.AddSnowflake(p.id)
		// Wait for a client to avail an offer to the snowflake, or timeout
		// and ask the snowflake to poll later.
		go func(p *ProxyRequest) {
			select {
			case offer := <-snowflake.offerChannel:
				log.Println("Passing client offer to snowflake.")
				p.offerChan <- offer
			case <-time.After(time.Second * ProxyTimeout):
				// This snowflake is no longer available to serve clients.
				heap.Remove(ctx.snowflakes, snowflake.index)
				delete(ctx.snowflakeMap, snowflake.id)
				p.offerChan <- nil
			}
		}(p)
	}
}

func robotsTxtHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte("User-agent: *\nDisallow:\n"))
}

func ipHandler(w http.ResponseWriter, r *http.Request) {
	remoteAddr := r.RemoteAddr
	if net.ParseIP(remoteAddr).To4() == nil {
		remoteAddr = "[" + remoteAddr + "]"
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(remoteAddr))
}

// Return early if it's CORS preflight.
func isPreflight(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Origin, X-Session-ID")
	if "OPTIONS" == r.Method {
		return true
	}
	return false
}

/*
Expects a WebRTC SDP offer in the Request to give to an assigned
snowflake proxy, which responds with the SDP answer to be sent in
the HTTP response back to the client.
*/
func clientHandler(ctx *BrokerContext, w http.ResponseWriter, r *http.Request) {
	offer, err := ioutil.ReadAll(r.Body)
	if nil != err {
		log.Println("Invalid data.")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "X-Session-ID")

	// Find the most available snowflake proxy, and pass the offer to it.
	// TODO: Needs improvement - maybe shouldn'
	if ctx.snowflakes.Len() <= 0 {
		log.Println("Client: No snowflake proxies available.")
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	snowflake := heap.Pop(ctx.snowflakes).(*Snowflake)
	snowflake.offerChannel <- offer

	// Wait for the answer to be returned on the channel.
	select {
	case answer := <-snowflake.answerChannel:
		log.Println("Client: Retrieving answer")
		w.Write(answer)
		// Only remove from the snowflake map once the answer is set.
		delete(ctx.snowflakeMap, snowflake.id)

	case <-time.After(time.Second * ClientTimeout):
		log.Println("Client: Timed out.")
		w.WriteHeader(http.StatusGatewayTimeout)
		w.Write([]byte("timed out waiting for answer!"))
	}
}

/*
For snowflake proxies to request a client from the Broker.
*/
func proxyHandler(ctx *BrokerContext, w http.ResponseWriter, r *http.Request) {
	if isPreflight(w, r) {
		return
	}
	id := r.Header.Get("X-Session-ID")
	body, err := ioutil.ReadAll(r.Body)
	if nil != err {
		log.Println("Invalid data.")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if string(body) != id { // Mismatched IDs!
		w.WriteHeader(http.StatusBadRequest)
	}
	// Maybe confirm that X-Session-ID is the same.
	log.Println("Received snowflake: ", id)

	p := new(ProxyRequest)
	p.id = id
	p.offerChan = make(chan []byte)
	ctx.createChan <- p

	// Wait for a client to avail an offer to the snowflake, or timeout
	// and ask the snowflake to poll later.
	offer := <-p.offerChan
	if nil == offer {
		log.Println("Proxy " + id + " did not receive a Client offer.")
		w.WriteHeader(http.StatusGatewayTimeout)
		return
	}
	log.Println("Passing client offer to snowflake.")
	w.Write(offer)
}

/*
Expects snowflake proxes which have previously successfully received
an offer from proxyHandler to respond with an answer in an HTTP POST,
which the broker will pass back to the original client.
*/
func answerHandler(ctx *BrokerContext, w http.ResponseWriter, r *http.Request) {
	if isPreflight(w, r) {
		return
	}
	id := r.Header.Get("X-Session-ID")
	snowflake, ok := ctx.snowflakeMap[id]
	if !ok || nil == snowflake {
		// The snowflake took too long to respond with an answer,
		// and the designated client is no longer around / recognized by the Broker.
		w.WriteHeader(http.StatusGone)
		return
	}
	body, err := ioutil.ReadAll(r.Body)
	if nil != err || nil == body || len(body) <= 0 {
		log.Println("Invalid data.")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	log.Println("Received answer: ", body)
	snowflake.answerChannel <- body
}

func debugHandler(ctx *BrokerContext, w http.ResponseWriter, r *http.Request) {
	s := fmt.Sprintf("current: %d", ctx.snowflakes.Len())
	w.Write([]byte(s))
}

func init() {
	ctx := NewBrokerContext()

	go ctx.Broker()

	http.HandleFunc("/robots.txt", robotsTxtHandler)
	http.HandleFunc("/ip", ipHandler)

	http.Handle("/client", SnowflakeHandler{ctx, clientHandler})
	http.Handle("/proxy", SnowflakeHandler{ctx, proxyHandler})
	http.Handle("/answer", SnowflakeHandler{ctx, answerHandler})
	http.Handle("/debug", SnowflakeHandler{ctx, debugHandler})
}
