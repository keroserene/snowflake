package event

import "github.com/pion/webrtc/v3"

type SnowflakeEvent interface {
	IsSnowflakeEvent()
	String() string
}

type EventOnOfferCreated struct {
	SnowflakeEvent
	WebRTCLocalDescription *webrtc.SessionDescription
	Error                  error
}

type EventOnBrokerRendezvous struct {
	SnowflakeEvent
	WebRTCRemoteDescription *webrtc.SessionDescription
	Error                   error
}

type EventOnSnowflakeConnected struct {
	SnowflakeEvent
}

type EventOnSnowflakeConnectionFailed struct {
	SnowflakeEvent
	Error error
}

type EventOnProxyConnectionOver struct {
	SnowflakeEvent
	InboundTraffic  int
	OutboundTraffic int
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
