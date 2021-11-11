// WebRTC rendezvous requires the exchange of SessionDescriptions between
// peers in order to establish a PeerConnection.

package snowflake_client

import (
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/messages"
	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/nat"
	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/util"
	"github.com/pion/webrtc/v3"
)

const (
	brokerErrorUnexpected string = "Unexpected error, no answer."
	readLimit                    = 100000 //Maximum number of bytes to be read from an HTTP response
)

// RendezvousMethod represents a way of communicating with the broker: sending
// an encoded client poll request (SDP offer) and receiving an encoded client
// poll response (SDP answer) in return. RendezvousMethod is used by
// BrokerChannel, which is in charge of encoding and decoding, and all other
// tasks that are independent of the rendezvous method.
type RendezvousMethod interface {
	Exchange([]byte) ([]byte, error)
}

// BrokerChannel uses a RendezvousMethod to communicate with the Snowflake broker.
// The BrokerChannel is responsible for encoding and decoding SDP offers and answers;
// RendezvousMethod is responsible for the exchange of encoded information.
type BrokerChannel struct {
	Rendezvous         RendezvousMethod
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

// NewBrokerChannel construct a new BrokerChannel, where:
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

	var rendezvous RendezvousMethod
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
		Rendezvous:         rendezvous,
		keepLocalAddresses: keepLocalAddresses,
		natType:            nat.NATUnknown,
	}, nil
}

// Negotiate uses a RendezvousMethod to send the client's WebRTC SDP offer
// and receive a snowflake proxy WebRTC SDP answer in return.
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

	// Do the exchange using our RendezvousMethod.
	encResp, err := bc.Rendezvous.Exchange(encReq)
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

// SetNATType sets the NAT type of the client so we can send it to the WebRTC broker.
func (bc *BrokerChannel) SetNATType(NATType string) {
	bc.lock.Lock()
	bc.natType = NATType
	bc.lock.Unlock()
	log.Printf("NAT Type: %s", NATType)
}

// WebRTCDialer implements the |Tongue| interface to catch snowflakes, using BrokerChannel.
type WebRTCDialer struct {
	*BrokerChannel
	webrtcConfig *webrtc.Configuration
	max          int
}

// NewWebRTCDialer constructs a new WebRTCDialer.
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

// Catch initializes a WebRTC Connection by signaling through the BrokerChannel.
func (w WebRTCDialer) Catch() (*WebRTCPeer, error) {
	// TODO: [#25591] Fetch ICE server information from Broker.
	// TODO: [#25596] Consider TURN servers here too.
	return NewWebRTCPeer(w.webrtcConfig, w.BrokerChannel)
}

// GetMax returns the maximum number of snowflakes to collect.
func (w WebRTCDialer) GetMax() int {
	return w.max
}
