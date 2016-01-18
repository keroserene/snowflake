// Exchange WebRTC SessionDescriptions over meek.
// Much of this source is extracted from meek-client.go.
package main

import (
	"bytes"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"

	"github.com/keroserene/go-webrtc"
)

// RequestInfo encapsulates all the configuration used for a request–response
// roundtrip, including variables that may come from SOCKS args or from the
// command line.
type RequestInfo struct {
	// What to put in the X-Session-ID header.
	// SessionID string
	// The URL to request.
	URL *url.URL
	// The Host header to put in the HTTP request (optional and may be
	// different from the host name in URL).
	Host string
}

func NewRequestInfo(meekUrl string, front string) *RequestInfo {
	info := new(RequestInfo)
	requestUrl, err := url.Parse(meekUrl)
	if nil != err {
		return nil
	}
	info.URL = requestUrl
	info.Host = front
	// info.URL.Host = front
	// info.Host = info.URL.Host
	return info
}

// Meek Signalling Channel
type MeekChannel struct {
	info *RequestInfo
	// Used to make all requests.
	transport http.Transport
}

func NewMeekChannel(info *RequestInfo) *MeekChannel {
	m := new(MeekChannel)
	// We make a copy of DefaultTransport because we want the default Dial
	// and TLSHandshakeTimeout settings. But we want to disable the default
	// ProxyFromEnvironment setting. Proxy is overridden below if
	// options.ProxyURL is set.
	m.transport = *http.DefaultTransport.(*http.Transport)
	m.transport.Proxy = nil
	m.info = info
	return m
}

// Do an HTTP roundtrip using the payload data in buf.
func (m *MeekChannel) roundTripHTTP(buf []byte) (*http.Response, error) {
	// Compose an innocent looking request.
	req, err := http.NewRequest("POST", m.info.Host+"/reg/123", bytes.NewReader(buf))
	if nil != err {
		return nil, err
	}
	// Set actually desired target host.
	req.Host = m.info.URL.String()
	// req.Header.Set("X-Session-Id", m.info.SessionID)
	return m.transport.RoundTrip(req)
}

// Send an SDP offer to the meek facilitator, and wait for an SDP answer from
// the assigned proxy in the response.
func (m *MeekChannel) Negotiate(offer *webrtc.SessionDescription) (
	*webrtc.SessionDescription, error) {
	buf := []byte(offer.Serialize())
	resp, err := m.roundTripHTTP(buf)
	if nil != err {
		return nil, err
	}
	defer resp.Body.Close()
	log.Println("MeekChannel Response: ", resp)
	body, err := ioutil.ReadAll(resp.Body)
	if nil != err {
		return nil, err
	}
	log.Println("MeekChannel Body: ", string(body))
	answer := webrtc.DeserializeSessionDescription(string(body))
	return answer, nil
}

// Simple interim non-fronting HTTP POST negotiation, to be removed when more
// general fronting is present.
func sendOfferHTTP(url string, offer *webrtc.SessionDescription) (*webrtc.SessionDescription, error) {
	resp, err := http.Post(url, "", bytes.NewBuffer([]byte(offer.Serialize())))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	answer := webrtc.DeserializeSessionDescription(string(body))
	return answer, nil
}
