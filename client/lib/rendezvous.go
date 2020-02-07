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
	"net"
	"net/http"
	"net/url"

	"github.com/pion/sdp"
	"github.com/pion/webrtc"
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
	Host               string
	url                *url.URL
	transport          http.RoundTripper // Used to make all requests.
	keepLocalAddresses bool
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
func NewBrokerChannel(broker string, front string, transport http.RoundTripper, keepLocalAddresses bool) (*BrokerChannel, error) {
	targetURL, err := url.Parse(broker)
	if err != nil {
		return nil, err
	}
	log.Println("Rendezvous using Broker at:", broker)
	bc := new(BrokerChannel)
	bc.url = targetURL
	if front != "" { // Optional front domain.
		log.Println("Domain fronting using:", front)
		bc.Host = bc.url.Host
		bc.url.Host = front
	}

	bc.transport = transport
	bc.keepLocalAddresses = keepLocalAddresses
	return bc, nil
}

func limitedRead(r io.Reader, limit int64) ([]byte, error) {
	p, err := ioutil.ReadAll(&io.LimitedReader{R: r, N: limit + 1})
	if err != nil {
		return p, err
	} else if int64(len(p)) == limit+1 {
		return p[0:limit], io.ErrUnexpectedEOF
	}
	return p, err
}

// Stolen from https://github.com/golang/go/pull/30278
func IsLocal(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		// Local IPv4 addresses are defined in https://tools.ietf.org/html/rfc1918
		return ip4[0] == 10 ||
			(ip4[0] == 172 && ip4[1]&0xf0 == 16) ||
			(ip4[0] == 192 && ip4[1] == 168)
	}
	// Local IPv6 addresses are defined in https://tools.ietf.org/html/rfc4193
	return len(ip) == net.IPv6len && ip[0]&0xfe == 0xfc
}

// Removes local LAN address ICE candidates
func stripLocalAddresses(str string) string {
	var desc sdp.SessionDescription
	err := desc.Unmarshal([]byte(str))
	if err != nil {
		return str
	}
	for _, m := range desc.MediaDescriptions {
		attrs := make([]sdp.Attribute, 0)
		for _, a := range m.Attributes {
			if a.IsICECandidate() {
				ice, err := a.ToICECandidate()
				if err != nil {
					attrs = append(attrs, a)
					continue
				}
				if ice.Typ == "host" {
					ip := net.ParseIP(ice.Address)
					if ip == nil {
						attrs = append(attrs, a)
						continue
					}
					if IsLocal(ip) || ip.IsUnspecified() || ip.IsLoopback() {
						continue
					}
				}
			}
			attrs = append(attrs, a)
		}
		m.Attributes = attrs
	}
	bts, err := desc.Marshal()
	if err != nil {
		return str
	}
	return string(bts)
}

// Roundtrip HTTP POST using WebRTC SessionDescriptions.
//
// Send an SDP offer to the broker, which assigns a proxy and responds
// with an SDP answer from a designated remote WebRTC peer.
func (bc *BrokerChannel) Negotiate(offer *webrtc.SessionDescription) (
	*webrtc.SessionDescription, error) {
	log.Println("Negotiating via BrokerChannel...\nTarget URL: ",
		bc.Host, "\nFront URL:  ", bc.url.Host)
	// Ideally, we could specify an `RTCIceTransportPolicy` that would handle
	// this for us.  However, "public" was removed from the draft spec.
	// See https://developer.mozilla.org/en-US/docs/Web/API/RTCConfiguration#RTCIceTransportPolicy_enum
	if !bc.keepLocalAddresses {
		offer = &webrtc.SessionDescription{
			Type: offer.Type,
			SDP:  stripLocalAddresses(offer.SDP),
		}
	}
	data := bytes.NewReader([]byte(serializeSessionDescription(offer)))
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
		answer := deserializeSessionDescription(string(body))
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

func NewWebRTCDialer(broker *BrokerChannel, iceServers []webrtc.ICEServer) *WebRTCDialer {
	config := webrtc.Configuration{
		ICEServers: iceServers,
	}
	return &WebRTCDialer{
		BrokerChannel: broker,
		webrtcConfig:  &config,
	}
}

// Initialize a WebRTC Connection by signaling through the broker.
func (w WebRTCDialer) Catch() (Snowflake, error) {
	// TODO: [#3] Fetch ICE server information from Broker.
	// TODO: [#18] Consider TURN servers here too.
	connection := NewWebRTCPeer(w.webrtcConfig, w.BrokerChannel)
	err := connection.Connect()
	return connection, err
}
