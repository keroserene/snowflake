package snowflake_broker

import (
	"container/heap"
	// "fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"time"
	// "appengine"
	// "appengine/urlfetch"
)

// This is an intermediate step - a basic hardcoded appengine rendezvous
// to a single browser snowflake.

// This is minimum viable client-proxy registration.
// TODO: better, more secure registration corresponding to what's in
// the python flashproxy facilitator.

// Slice of available snowflake proxies.
// var snowflakes []chan []byte

type Snowflake struct {
	id         string
	sigChannel chan []byte
	clients    int
	index      int
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

// Create and add a Snowflake to the heap.
func AddSnowflake(id string) *Snowflake {
	snowflake := new(Snowflake)
	snowflake.id = id
	snowflake.clients = 0
	snowflake.sigChannel = make(chan []byte)
	heap.Push(snowflakes, snowflake)
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
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	// Pop the most available snowflake proxy, and pass the offer to it.
	// TODO: Make this much better.
	snowflake := heap.Pop(snowflakes).(*Snowflake)
	if nil == snowflake {
		// w.Header().Set("Status", http.StatusServiceUnavailable)
		w.Write([]byte("no snowflake proxies available"))
		return
	}
	// snowflakes = snowflakes[1:]
	snowflake.sigChannel <- offer
	w.Write([]byte("sent offer to proxy!"))
	// TODO: Get browser snowflake to talkto this appengine instance
	// so it can reply with an answer, and not just the offer again :)
	// TODO: Real broker which matches clients and snowflake proxies.
	w.Write(offer)
}

/*
A snowflake browser proxy requests a client from the Broker.
*/
func proxyHandler(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if nil != err {
		log.Println("Invalid data.")
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	snowflakeSession := body
	log.Println("Received snowflake: ", snowflakeSession)
	snowflake := AddSnowflake(string(snowflakeSession))
	select {
	case offer := <-snowflake.sigChannel:
		log.Println("Passing client offer to snowflake.")
		w.Write(offer)
	case <-time.After(time.Second * 10):
		// s := fmt.Sprintf("%d snowflakes left.", snowflakes.Len())
		// w.Write([]byte("timed out. " + s))
		// w.Header().Set("Status", http.StatusRequestTimeout)
		w.WriteHeader(http.StatusGatewayTimeout)
		heap.Remove(snowflakes, snowflake.index)
	}
}

func reflectHandler(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if nil != err {
		log.Println("Invalid data.")
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write(body)
}

func init() {
	// snowflakes = make([]chan []byte, 0)
	snowflakes = new(SnowflakeHeap)
	heap.Init(snowflakes)

	http.HandleFunc("/robots.txt", robotsTxtHandler)
	http.HandleFunc("/ip", ipHandler)

	http.HandleFunc("/client", clientHandler)
	http.HandleFunc("/proxy", proxyHandler)
	http.HandleFunc("/reflect", reflectHandler)
	// if SNOWFLAKE_BROKER == "" {
	// panic("SNOWFLAKE_BROKER empty; did you forget to edit config.go?")
	// }
}
