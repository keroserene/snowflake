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
	// "fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"time"
)

// This is an intermediate step - a basic hardcoded appengine rendezvous
// to a single browser snowflake.

// This is minimum viable client-proxy registration.
// TODO: better, more secure registration corresponding to what's in
// the python flashproxy facilitator.

// Slice of available snowflake proxies.
// var snowflakes []chan []byte

type Snowflake struct {
	id            string
	offerChannel  chan []byte
	answerChannel chan []byte
	clients       int
	index         int
}

// Implements heap.Interface, and holds Snowflakes.
type SnowflakeHeap []*Snowflake

func (sh SnowflakeHeap) Len() int { return len(sh) }

func (sh SnowflakeHeap) Less(i, j int) bool {
	// Snowflakes serving less clients should sort earlier.
	return sh[i].clients < sh[j].clients
}

func (sh SnowflakeHeap) Swap(i, j int) {
	sh[i], sh[j] = sh[j], sh[i]
	sh[i].index = i
	sh[j].index = j
}

func (sh *SnowflakeHeap) Push(s interface{}) {
	n := len(*sh)
	snowflake := s.(*Snowflake)
	snowflake.index = n
	*sh = append(*sh, snowflake)
}

// Only valid when Len() > 0.
func (sh *SnowflakeHeap) Pop() interface{} {
	flakes := *sh
	n := len(flakes)
	snowflake := flakes[n-1]
	snowflake.index = -1
	*sh = flakes[0 : n-1]
	return snowflake
}

var snowflakes *SnowflakeHeap
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
	// TODO: Needs improvement.
	snowflake := heap.Pop(snowflakes).(*Snowflake)
	if nil == snowflake {
		w.Header().Set("Status", http.StatusServiceUnavailable)
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

	case <-time.After(time.Second * 10):
		w.WriteHeader(http.StatusGatewayTimeout)
		w.Write([]byte("timed out waiting for answer!"))
	}
}

/*
For snowflake proxies to request a client from the Broker.
*/
func proxyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Origin, X-Session-ID")
	// For CORS preflight.
	if "OPTIONS" == r.Method {
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

	case <-time.After(time.Second * 10):
		heap.Remove(snowflakes, snowflake.index)
		w.WriteHeader(http.StatusGatewayTimeout)
	}
}

/*
Expects snowflake proxes which have previously successfully received
an offer from proxyHandler to respond with an answer in an HTTP POST,
which the broker will pass back to the original client.
*/
func answerHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "X-Session-ID")
	// For CORS preflight.
	if "OPTIONS" == r.Method {
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
