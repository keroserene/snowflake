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

// This is minimum viable client-proxy registration.
// TODO(#13): better, more secure registration corresponding to what's in
// the python flashproxy facilitator.
var snowflakes *SnowflakeHeap

// Map keeping track of snowflakeIDs required to match SDP answers from
// the second http POST.
var snowflakeMap map[string]*Snowflake

// Create and add a Snowflake to the heap.
func AddSnowflake(id string) *Snowflake {
	snowflake := new(Snowflake)
	snowflake.id = id
	snowflake.clients = 0
	snowflake.offerChannel = make(chan []byte)
	snowflake.answerChannel = make(chan []byte)
	heap.Push(snowflakes, snowflake)
	snowflakeMap[id] = snowflake
	return snowflake
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
func clientHandler(w http.ResponseWriter, r *http.Request) {
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
	snowflake := heap.Pop(snowflakes).(*Snowflake)
	if nil == snowflake {
		w.WriteHeader(http.StatusServiceUnavailable)
		// w.Write([]byte("no snowflake proxies available"))
		return
	}
	snowflake.offerChannel <- offer

	// Wait for the answer to be returned on the channel.
	select {
	case answer := <-snowflake.answerChannel:
		log.Println("Retrieving answer")
		w.Write(answer)
		// Only remove from the snowflake map once the answer is set.
		delete(snowflakeMap, snowflake.id)

	case <-time.After(time.Second * ClientTimeout):
		w.WriteHeader(http.StatusGatewayTimeout)
		w.Write([]byte("timed out waiting for answer!"))
	}
}

/*
For snowflake proxies to request a client from the Broker.
*/
func proxyHandler(w http.ResponseWriter, r *http.Request) {
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
	snowflake := AddSnowflake(id)

	// Wait for a client to avail an offer to the snowflake, or timeout
	// and ask the snowflake to poll later.
	select {
	case offer := <-snowflake.offerChannel:
		log.Println("Passing client offer to snowflake.")
		w.Write(offer)

	case <-time.After(time.Second * ProxyTimeout):
		// This snowflake is no longer available to serve clients.
		heap.Remove(snowflakes, snowflake.index)
		delete(snowflakeMap, snowflake.id)
		w.WriteHeader(http.StatusGatewayTimeout)
	}
}

/*
Expects snowflake proxes which have previously successfully received
an offer from proxyHandler to respond with an answer in an HTTP POST,
which the broker will pass back to the original client.
*/
func answerHandler(w http.ResponseWriter, r *http.Request) {
	if isPreflight(w, r) {
		return
	}
	id := r.Header.Get("X-Session-ID")
	snowflake, ok := snowflakeMap[id]
	if !ok || nil == snowflake {
		// The snowflake took too long to respond with an answer,
		// and the designated client is no longer around / recognized by the Broker.
		w.WriteHeader(http.StatusGone)
		return
	}
	body, err := ioutil.ReadAll(r.Body)
	if nil != err {
		log.Println("Invalid data.")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	log.Println("Received answer: ", body)
	snowflake.answerChannel <- body
}

func init() {
	snowflakes = new(SnowflakeHeap)
	snowflakeMap = make(map[string]*Snowflake)
	heap.Init(snowflakes)

	http.HandleFunc("/robots.txt", robotsTxtHandler)
	http.HandleFunc("/ip", ipHandler)

	http.HandleFunc("/client", clientHandler)
	http.HandleFunc("/proxy", proxyHandler)
	http.HandleFunc("/answer", answerHandler)
}
