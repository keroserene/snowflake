// WebRTC rendezvous requires the exchange of SessionDescriptions between
// peers in order to establish a PeerConnection.
//
// This file contains the one method currently available to Snowflake:
//
// - Domain-fronted HTTP signaling. The Broker automatically exchange offers
//   and answers between this client and some remote WebRTC proxy.

package lib

import (
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"git.torproject.org/pluggable-transports/snowflake.git/common/messages"
	"git.torproject.org/pluggable-transports/snowflake.git/common/nat"
	"git.torproject.org/pluggable-transports/snowflake.git/common/util"
	"github.com/pion/webrtc/v3"
)

const (
	BrokerErrorUnexpected string = "Unexpected error, no answer."
	readLimit                    = 100000 //Maximum number of bytes to be read from an HTTP response
)

// rendezvousMethod represents a way of communicating with the broker: sending
// an encoded client poll request (SDP offer) and receiving an encoded client
// poll response (SDP answer) in return. rendezvousMethod is used by
// BrokerChannel, which is in charge of encoding and decoding, and all other
// tasks that are independent of the rendezvous method.
type rendezvousMethod interface {
	Exchange([]byte) ([]byte, error)
}

// BrokerChannel contains a rendezvousMethod, as well as data that is not
// specific to any rendezvousMethod. BrokerChannel has the responsibility of
// encoding and decoding SDP offers and answers; rendezvousMethod is responsible
// for the exchange of encoded information.
type BrokerChannel struct {
	rendezvous         rendezvousMethod
	keepLocalAddresses bool
	natType            string
	lock               sync.Mutex
}

// We make a copy of DefaultTransport because we want the default Dial
// and TLSHandshakeTimeout settings. But we want to disable the default
// ProxyFromEnvironment setting.
func createBrokerTransport() http.RoundTripper {
	transport := http.DefaultTransport.(*http.Transport)
	transport.Proxy = nil
	transport.ResponseHeaderTimeout = 15 * time.Second
	return transport
}

// Construct a new BrokerChannel, where:
// |broker| is the full URL of the facilitating program which assigns proxies
// to clients, and |front| is the option fronting domain.
func NewBrokerChannel(broker, ampCache, front string, keepLocalAddresses bool) (*BrokerChannel, error) {
	log.Println("Rendezvous using Broker at:", broker)
	if ampCache != "" {
		log.Println("Through AMP cache at:", ampCache)
	}
	if front != "" {
		log.Println("Domain fronting using:", front)
	}

	var rendezvous rendezvousMethod
	var err error
	if ampCache != "" {
		rendezvous, err = newAMPCacheRendezvous(broker, ampCache, front, createBrokerTransport())
	} else {
		rendezvous, err = newHTTPRendezvous(broker, front, createBrokerTransport())
	}
	if err != nil {
		return nil, err
	}

	return &BrokerChannel{
		rendezvous:         rendezvous,
		keepLocalAddresses: keepLocalAddresses,
		natType:            nat.NATUnknown,
	}, nil
}

// Roundtrip HTTP POST using WebRTC SessionDescriptions.
//
// Send an SDP offer to the broker, which assigns a proxy and responds
// with an SDP answer from a designated remote WebRTC peer.
func (bc *BrokerChannel) Negotiate(offer *webrtc.SessionDescription) (
	*webrtc.SessionDescription, error) {
	// Ideally, we could specify an `RTCIceTransportPolicy` that would handle
	// this for us.  However, "public" was removed from the draft spec.
	// See https://developer.mozilla.org/en-US/docs/Web/API/RTCConfiguration#RTCIceTransportPolicy_enum
	if !bc.keepLocalAddresses {
		offer = &webrtc.SessionDescription{
			Type: offer.Type,
			SDP:  util.StripLocalAddresses(offer.SDP),
		}
	}
	offerSDP, err := util.SerializeSessionDescription(offer)
	if err != nil {
		return nil, err
	}

	// Encode the client poll request.
	bc.lock.Lock()
	req := &messages.ClientPollRequest{
		Offer: offerSDP,
		NAT:   bc.natType,
	}
	encReq, err := req.EncodePollRequest()
	bc.lock.Unlock()
	if err != nil {
		return nil, err
	}

	// Do the exchange using our rendezvousMethod.
	encResp, err := bc.rendezvous.Exchange(encReq)
	if err != nil {
		return nil, err
	}
	log.Printf("Received answer: %s", string(encResp))

	// Decode the client poll response.
	resp, err := messages.DecodeClientPollResponse(encResp)
	if err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, errors.New(resp.Error)
	}
	return util.DeserializeSessionDescription(resp.Answer)
}

func (bc *BrokerChannel) SetNATType(NATType string) {
	bc.lock.Lock()
	bc.natType = NATType
	bc.lock.Unlock()
	log.Printf("NAT Type: %s", NATType)
}

// Implements the |Tongue| interface to catch snowflakes, using BrokerChannel.
type WebRTCDialer struct {
	*BrokerChannel
	webrtcConfig *webrtc.Configuration
	max          int
}

func NewWebRTCDialer(broker *BrokerChannel, iceServers []webrtc.ICEServer, max int) *WebRTCDialer {
	config := webrtc.Configuration{
		ICEServers: iceServers,
	}

	return &WebRTCDialer{
		BrokerChannel: broker,
		webrtcConfig:  &config,
		max:           max,
	}
}

// Initialize a WebRTC Connection by signaling through the broker.
func (w WebRTCDialer) Catch() (*WebRTCPeer, error) {
	// TODO: [#25591] Fetch ICE server information from Broker.
	// TODO: [#25596] Consider TURN servers here too.
	return NewWebRTCPeer(w.webrtcConfig, w.BrokerChannel)
}

// Returns the maximum number of snowflakes to collect
func (w WebRTCDialer) GetMax() int {
	return w.max
}
