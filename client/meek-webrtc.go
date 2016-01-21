// Exchange WebRTC SessionDescriptions over a domain-fronted HTTP
// signaling channel.
package main

import (
	"bytes"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"

	"github.com/keroserene/go-webrtc"
)

// Meek Signalling Channel.
type MeekChannel struct {
	// The Host header to put in the HTTP request (optional and may be
	// different from the host name in URL).
	Host        string
	Method      string
	trueURL     *url.URL
	externalUrl string
	transport   http.Transport // Used to make all requests.
}

// Construct a new MeekChannel, where
// |broker| is the URL of the facilitating program which assigns proxies
// to clients, and
// |front| is URL of the front domain.
func NewMeekChannel(broker string, front string) *MeekChannel {
	targetUrl, err := url.Parse(broker)
	if nil != err {
		return nil
	}
	mc := new(MeekChannel)
	mc.Host = front
	mc.Method = "POST"

	mc.trueURL = targetUrl
	mc.externalUrl = front + "/client"

	// We make a copy of DefaultTransport because we want the default Dial
	// and TLSHandshakeTimeout settings. But we want to disable the default
	// ProxyFromEnvironment setting.
	mc.transport = *http.DefaultTransport.(*http.Transport)
	mc.transport.Proxy = nil
	return mc
}

// Roundtrip HTTP POST using WebRTC SessionDescriptions.
//
// Sends an SDP offer to the meek broker, which assigns a proxy and responds
// with an SDP answer from a designated remote WebRTC peer.
func (mc *MeekChannel) Negotiate(offer *webrtc.SessionDescription) (
	*webrtc.SessionDescription, error) {
	data := bytes.NewReader([]byte(offer.Serialize()))
	request, err := http.NewRequest(mc.Method, mc.externalUrl, data)
	if nil != err {
		return nil, err
	}
	request.Host = mc.trueURL.String()
	resp, err := mc.transport.RoundTrip(request)
	if nil != err {
		return nil, err
	}
	defer resp.Body.Close()
	log.Println("MeekChannel Response: ", resp)

	body, err := ioutil.ReadAll(resp.Body)
	if nil != err {
		return nil, err
	}
	log.Println("Body: ", string(body))
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
