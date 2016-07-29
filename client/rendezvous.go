// WebRTC rendezvous requires the exchange of SessionDescriptions between
// peers in order to establish a PeerConnection.
//
// This file contains the two methods currently available to Snowflake:
//
// - Domain-fronted HTTP signaling. The Broker automatically exchange offers
//   and answers between this client and some remote WebRTC proxy.
//   (This is the recommended default, enabled via the flags in "torrc".)
//
// - Manual copy-paste signaling. User must create a signaling pipe.
//   (The flags in torrc-manual allow this)
package main

import (
	"bufio"
	"bytes"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"syscall"

	"github.com/keroserene/go-webrtc"
)

const (
	BrokerError503        string = "No snowflake proxies currently available."
	BrokerError400        string = "You sent an invalid offer in the request."
	BrokerErrorUnexpected string = "Unexpected error, no answer."
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

// CopyPasteDialer handles the interaction required to copy-paste the
// offers and answers.
// Implements |Tongue| interface to catch snowflakes manually.
// Supports recovery of connections.
type CopyPasteDialer struct {
	webrtcConfig *webrtc.Configuration
	signal       *os.File
	current      *WebRTCPeer
}

func NewCopyPasteDialer(iceServers IceServerList) *CopyPasteDialer {
	log.Println("No HTTP signaling detected. Using manual copy-paste signaling.")
	log.Println("Waiting for a \"signal\" pipe...")
	// This FIFO receives signaling messages.
	err := syscall.Mkfifo("signal", 0600)
	if err != nil {
		if syscall.EEXIST != err.(syscall.Errno) {
			log.Fatal(err)
		}
	}
	signalFile, err := os.OpenFile("signal", os.O_RDONLY, 0600)
	if nil != err {
		log.Fatal(err)
		return nil
	}
	config := webrtc.NewConfiguration(iceServers...)
	dialer := &CopyPasteDialer{
		webrtcConfig: config,
		signal:       signalFile,
	}
	go dialer.readSignals()
	return dialer
}

// Initialize a WebRTC Peer via manual copy-paste.
func (d *CopyPasteDialer) Catch() (Snowflake, error) {
	if nil == d.signal {
		return nil, errors.New("Cannot copy-paste dial without signal pipe.")
	}
	connection := NewWebRTCPeer(d.webrtcConfig, nil)
	// Must keep track of pending new connection until copy-paste completes.
	d.current = connection
	// Outputs SDP offer to log, expecting user to copy-paste to the remote Peer.
	// Blocks until user pastes back the answer.
	err := connection.Connect()
	d.current = nil
	return connection, err
}

// Manual copy-paste signalling.
func (d *CopyPasteDialer) readSignals() {
	defer d.signal.Close()
	log.Printf("CopyPasteDialer: reading messages from signal pipe.")
	s := bufio.NewScanner(d.signal)
	for s.Scan() {
		msg := s.Text()
		sdp := webrtc.DeserializeSessionDescription(msg)
		if sdp == nil {
			log.Printf("CopyPasteDialer: ignoring invalid signal message %+q", msg)
			continue
		}
		d.current.answerChannel <- sdp
	}
	if err := s.Err(); err != nil {
		log.Printf("signal FIFO: %s", err)
	}
}
