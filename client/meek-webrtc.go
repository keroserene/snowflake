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
	Host      string
	url       *url.URL
	transport http.Transport // Used to make all requests.
}

// Construct a new MeekChannel, where:
// |broker| is the full URL of the facilitating program which assigns proxies
// to clients, and |front| is the option fronting domain.
func NewMeekChannel(broker string, front string) *MeekChannel {
	targetURL, err := url.Parse(broker)
	if nil != err {
		return nil
	}
	mc := new(MeekChannel)
	mc.url = targetURL
	if "" != front { // Optional front domain.
		mc.Host = mc.url.Host
		mc.url.Host = front
	}

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
	// Suffix with broker's client registration handler.
	request, err := http.NewRequest("POST", mc.url.String()+"client", data)
	if nil != err {
		return nil, err
	}
	if "" != mc.Host { // Set true host if necessary.
		request.Host = mc.Host
	}
	resp, err := mc.transport.RoundTrip(request)
	if nil != err {
		return nil, err
	}
	defer resp.Body.Close()
	log.Printf("MeekChannel Response:\n%s\n\n", resp)
	body, err := ioutil.ReadAll(resp.Body)
	if nil != err {
		return nil, err
	}
	answer := webrtc.DeserializeSessionDescription(string(body))
	return answer, nil
}
