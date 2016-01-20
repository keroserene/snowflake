package snowflake_broker

import (
	// "io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"path"

	// "appengine"
	// "appengine/urlfetch"
)

// This is an intermediate step - a basic hardcoded appengine rendezvous
// to a single browser snowflake.

var snowflakeProxy = ""

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
func regHandler(w http.ResponseWriter, r *http.Request) {
	// TODO: Maybe don't pass anything on path, since it will always be bidirectional
	dir, _ := path.Split(path.Clean(r.URL.Path))
	if dir != "/reg/" {
		http.NotFound(w, r)
		return
	}
	body, err := ioutil.ReadAll(r.Body)
	if nil != err {
		return
		log.Println("Invalid data.")
	}

	// TODO: Get browser snowflake to talkto this appengine instance
	// so it can reply with an answer, and not just the offer again :)
	// TODO: Real facilitator which matches clients and snowflake proxies.
	w.Write(body)
}

func init() {
	http.HandleFunc("/robots.txt", robotsTxtHandler)
	http.HandleFunc("/ip", ipHandler)
	http.HandleFunc("/reg/", regHandler)
	// if SNOWFLAKE_FACILITATOR == "" {
	// panic("SNOWFLAKE_FACILITATOR empty; did you forget to edit config.go?")
	// }
}
