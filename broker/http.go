package main

import (
	"errors"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/messages"
)

const (
	readLimit = 100000 // Maximum number of bytes to be read from an HTTP request
)

// Implements the http.Handler interface
type SnowflakeHandler struct {
	*IPC
	handle func(*IPC, http.ResponseWriter, *http.Request)
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

// Implements the http.Handler interface
type MetricsHandler struct {
	logFilename string
	handle      func(string, http.ResponseWriter, *http.Request)
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

/*
For snowflake proxies to request a client from the Broker.
*/
func proxyPolls(i *IPC, w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(http.MaxBytesReader(w, r.Body, readLimit))
	if err != nil {
		log.Println("Invalid data.", err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	arg := messages.Arg{
		Body:       body,
		RemoteAddr: r.RemoteAddr,
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

	// Handle the legacy version
	//
	// We support two client message formats. The legacy format is for backwards
	// combatability and relies heavily on HTTP headers and status codes to convey
	// information.
	isLegacy := false
	if len(body) > 0 && body[0] == '{' {
		isLegacy = true
		req := messages.ClientPollRequest{
			Offer: string(body),
			NAT:   r.Header.Get("Snowflake-NAT-Type"),
		}
		body, err = req.EncodeClientPollRequest()
		if err != nil {
			log.Printf("Error shimming the legacy request: %s", err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	arg := messages.Arg{
		Body:       body,
		RemoteAddr: "",
	}

	var response []byte
	err = i.ClientOffers(arg, &response)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if isLegacy {
		resp, err := messages.DecodeClientPollResponse(response)
		if err != nil {
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		switch resp.Error {
		case "":
			response = []byte(resp.Answer)
		case messages.StrNoProxies:
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		case messages.StrTimedOut:
			w.WriteHeader(http.StatusGatewayTimeout)
			return
		default:
			panic("unknown error")
		}
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
		log.Println("Invalid data.", err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	arg := messages.Arg{
		Body:       body,
		RemoteAddr: "",
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
