package event

import (
	"fmt"

	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/safelog"
	"github.com/pion/webrtc/v3"
)

type SnowflakeEvent interface {
	IsSnowflakeEvent()
	String() string
}

type EventOnOfferCreated struct {
	SnowflakeEvent
	WebRTCLocalDescription *webrtc.SessionDescription
	Error                  error
}

func (e EventOnOfferCreated) String() string {
	if e.Error != nil {
		scrubbed := safelog.Scrub([]byte(e.Error.Error()))
		return fmt.Sprintf("offer creation failure %s", scrubbed)
	}
	return "offer created"
}

type EventOnBrokerRendezvous struct {
	SnowflakeEvent
	WebRTCRemoteDescription *webrtc.SessionDescription
	Error                   error
}

func (e EventOnBrokerRendezvous) String() string {
	if e.Error != nil {
		scrubbed := safelog.Scrub([]byte(e.Error.Error()))
		return fmt.Sprintf("broker failure %s", scrubbed)
	}
	return "broker rendezvous peer received"
}

type EventOnSnowflakeConnected struct {
	SnowflakeEvent
}

func (e EventOnSnowflakeConnected) String() string {
	return "connected"
}

type EventOnSnowflakeConnectionFailed struct {
	SnowflakeEvent
	Error error
}

func (e EventOnSnowflakeConnectionFailed) String() string {
	scrubbed := safelog.Scrub([]byte(e.Error.Error()))
	return fmt.Sprintf("trying a new proxy: %s", scrubbed)
}

type EventOnProxyConnectionOver struct {
	SnowflakeEvent
	InboundTraffic  int
	OutboundTraffic int
}

func (e EventOnProxyConnectionOver) String() string {
	return fmt.Sprintf("Proxy connection closed (↑ %d, ↓ %d)", e.InboundTraffic, e.OutboundTraffic)
}

type SnowflakeEventReceiver interface {
	// OnNewSnowflakeEvent notify receiver about a new event
	// This method MUST not block
	OnNewSnowflakeEvent(event SnowflakeEvent)
}

type SnowflakeEventDispatcher interface {
	SnowflakeEventReceiver
	// AddSnowflakeEventListener allow receiver(s) to receive event notification
	// when OnNewSnowflakeEvent is called on the dispatcher.
	// Every event listener added will be called when an event is received by the dispatcher.
	// The order each listener is called is undefined.
	AddSnowflakeEventListener(receiver SnowflakeEventReceiver)
	RemoveSnowflakeEventListener(receiver SnowflakeEventReceiver)
}
