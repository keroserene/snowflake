// WebRTC Rendezvous requires the exchange of SessionDescriptions between
// peers. This file contains the domain-fronted HTTP signaling mechanism
// between the client and a desired Broker.
package main

import (
	"bytes"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"

	"github.com/keroserene/go-webrtc"
)

// Signalling Channel to the Broker.
type BrokerChannel struct {
	// The Host header to put in the HTTP request (optional and may be
	// different from the host name in URL).
	Host string
	url  *url.URL
	transport http.RoundTripper // Used to make all requests.
}

// Construct a new BrokerChannel, where:
// |broker| is the full URL of the facilitating program which assigns proxies
// to clients, and |front| is the option fronting domain.
func NewBrokerChannel(broker string, front string) *BrokerChannel {
	targetURL, err := url.Parse(broker)
	if nil != err {
		return nil
	}
	bc := new(BrokerChannel)
	bc.url = targetURL
	if "" != front { // Optional front domain.
		bc.Host = bc.url.Host
		bc.url.Host = front
	}

	// We make a copy of DefaultTransport because we want the default Dial
	// and TLSHandshakeTimeout settings. But we want to disable the default
	// ProxyFromEnvironment setting.
	transport := http.DefaultTransport.(*http.Transport)
	transport.Proxy = nil
	bc.transport = transport
	return bc
}

// Roundtrip HTTP POST using WebRTC SessionDescriptions.
//
// Send an SDP offer to the broker, which assigns a proxy and responds
// with an SDP answer from a designated remote WebRTC peer.
func (bc *BrokerChannel) Negotiate(offer *webrtc.SessionDescription) (
	*webrtc.SessionDescription, error) {
	data := bytes.NewReader([]byte(offer.Serialize()))
	// Suffix with broker's client registration handler.
	request, err := http.NewRequest("POST", bc.url.String()+"client", data)
	if nil != err {
		return nil, err
	}
	if "" != bc.Host { // Set true host if necessary.
		request.Host = bc.Host
	}
	resp, err := bc.transport.RoundTrip(request)
	if nil != err {
		return nil, err
	}
	defer resp.Body.Close()
	log.Printf("BrokerChannel Response:\n%s\n\n", resp.Status)

	switch resp.StatusCode {
	case http.StatusOK:
		body, err := ioutil.ReadAll(resp.Body)
		if nil != err {
			return nil, err
		}
		answer := webrtc.DeserializeSessionDescription(string(body))
		return answer, nil

	case http.StatusServiceUnavailable:
		return nil, errors.New("No snowflake proxies currently available.")
	case http.StatusBadRequest:
		return nil, errors.New("You sent an invalid offer in the request.")
	default:
		return nil, errors.New("Unexpected error, no answer.")
	}
}
