// WebRTC rendezvous requires the exchange of SessionDescriptions between
// peers in order to establish a PeerConnection.
//
// This file contains the one method currently available to Snowflake:
//
// - Domain-fronted HTTP signaling. The Broker automatically exchange offers
//   and answers between this client and some remote WebRTC proxy.

package lib

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"

	"github.com/keroserene/go-webrtc"
)

const (
	BrokerError503        string = "No snowflake proxies currently available."
	BrokerError400        string = "You sent an invalid offer in the request."
	BrokerErrorUnexpected string = "Unexpected error, no answer."
	readLimit                    = 100000 //Maximum number of bytes to be read from an HTTP response
)

// Signalling Channel to the Broker.
type BrokerChannel struct {
	// The Host header to put in the HTTP request (optional and may be
	// different from the host name in URL).
	Host      string
	url       *url.URL
	transport http.RoundTripper // Used to make all requests.
}

// We make a copy of DefaultTransport because we want the default Dial
// and TLSHandshakeTimeout settings. But we want to disable the default
// ProxyFromEnvironment setting.
func CreateBrokerTransport() http.RoundTripper {
	transport := http.DefaultTransport.(*http.Transport)
	transport.Proxy = nil
	return transport
}

// Construct a new BrokerChannel, where:
// |broker| is the full URL of the facilitating program which assigns proxies
// to clients, and |front| is the option fronting domain.
func NewBrokerChannel(broker string, front string, transport http.RoundTripper) *BrokerChannel {
	targetURL, err := url.Parse(broker)
	if nil != err {
		return nil
	}
	log.Println("Rendezvous using Broker at:", broker)
	bc := new(BrokerChannel)
	bc.url = targetURL
	if "" != front { // Optional front domain.
		log.Println("Domain fronting using:", front)
		bc.Host = bc.url.Host
		bc.url.Host = front
	}

	bc.transport = transport
	return bc
}

func limitedRead(r io.Reader, limit int64) ([]byte, error) {
	p, err := ioutil.ReadAll(&io.LimitedReader{r, limit})
	if err != nil {
		return p, err
	}

	//Check to see if limit was exceeded
	var tmp [1]byte
	_, err = io.ReadFull(r, tmp[:])
	if err == io.EOF {
		err = nil
	} else if err == nil {
		err = io.ErrUnexpectedEOF
	}
	return p, err
}

// Roundtrip HTTP POST using WebRTC SessionDescriptions.
//
// Send an SDP offer to the broker, which assigns a proxy and responds
// with an SDP answer from a designated remote WebRTC peer.
func (bc *BrokerChannel) Negotiate(offer *webrtc.SessionDescription) (
	*webrtc.SessionDescription, error) {
	log.Println("Negotiating via BrokerChannel...\nTarget URL: ",
		bc.Host, "\nFront URL:  ", bc.url.Host)
	data := bytes.NewReader([]byte(offer.Serialize()))
	// Suffix with broker's client registration handler.
	clientURL := bc.url.ResolveReference(&url.URL{Path: "client"})
	request, err := http.NewRequest("POST", clientURL.String(), data)
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
		body, err := limitedRead(resp.Body, readLimit)
		if nil != err {
			return nil, err
		}
		answer := webrtc.DeserializeSessionDescription(string(body))
		return answer, nil

	case http.StatusServiceUnavailable:
		return nil, errors.New(BrokerError503)
	case http.StatusBadRequest:
		return nil, errors.New(BrokerError400)
	default:
		return nil, errors.New(BrokerErrorUnexpected)
	}
}

// Implements the |Tongue| interface to catch snowflakes, using BrokerChannel.
type WebRTCDialer struct {
	*BrokerChannel
	webrtcConfig *webrtc.Configuration
}

func NewWebRTCDialer(
	broker *BrokerChannel, iceServers IceServerList) *WebRTCDialer {
	config := webrtc.NewConfiguration(iceServers...)
	if nil == config {
		log.Println("Unable to prepare WebRTC configuration.")
		return nil
	}
	return &WebRTCDialer{
		BrokerChannel: broker,
		webrtcConfig:  config,
	}
}

// Initialize a WebRTC Connection by signaling through the broker.
func (w WebRTCDialer) Catch() (Snowflake, error) {
	if nil == w.BrokerChannel {
		return nil, errors.New("Cannot Dial WebRTC without a BrokerChannel.")
	}
	// TODO: [#3] Fetch ICE server information from Broker.
	// TODO: [#18] Consider TURN servers here too.
	connection := NewWebRTCPeer(w.webrtcConfig, w.BrokerChannel)
	err := connection.Connect()
	return connection, err
}
