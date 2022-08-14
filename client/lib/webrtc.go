package snowflake_client

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"sync"
	"time"

	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/event"
	"github.com/pion/ice/v2"
	"github.com/pion/webrtc/v3"
)

// WebRTCPeer represents a WebRTC connection to a remote snowflake proxy.
//
// Each WebRTCPeer only ever has one DataChannel that is used as the peer's transport.
type WebRTCPeer struct {
	id        string
	pc        *webrtc.PeerConnection
	transport *webrtc.DataChannel

	recvPipe  *io.PipeReader
	writePipe *io.PipeWriter

	mu          sync.Mutex // protects the following:
	lastReceive time.Time

	open   chan struct{} // Channel to notify when datachannel opens
	closed chan struct{}

	once sync.Once // Synchronization for PeerConnection destruction

	bytesLogger  bytesLogger
	eventsLogger event.SnowflakeEventReceiver
}

func NewWebRTCPeer(config *webrtc.Configuration,
	broker *BrokerChannel) (*WebRTCPeer, error) {
	return NewWebRTCPeerWithEvents(config, broker, nil)
}

// NewWebRTCPeerWithEvents constructs a WebRTC PeerConnection to a snowflake proxy.
//
// The creation of the peer handles the signaling to the Snowflake broker, including
// the exchange of SDP information, the creation of a PeerConnection, and the establishment
// of a DataChannel to the Snowflake proxy.
func NewWebRTCPeerWithEvents(config *webrtc.Configuration,
	broker *BrokerChannel, eventsLogger event.SnowflakeEventReceiver) (*WebRTCPeer, error) {
	if eventsLogger == nil {
		eventsLogger = event.NewSnowflakeEventDispatcher()
	}

	connection := new(WebRTCPeer)
	{
		var buf [8]byte
		if _, err := rand.Read(buf[:]); err != nil {
			panic(err)
		}
		connection.id = "snowflake-" + hex.EncodeToString(buf[:])
	}
	connection.closed = make(chan struct{})

	// Override with something that's not NullLogger to have real logging.
	connection.bytesLogger = &bytesNullLogger{}

	// Pipes remain the same even when DataChannel gets switched.
	connection.recvPipe, connection.writePipe = io.Pipe()

	connection.eventsLogger = eventsLogger

	err := connection.connect(config, broker)
	if err != nil {
		connection.Close()
		return nil, err
	}
	return connection, nil
}

// Read bytes from local SOCKS.
// As part of |io.ReadWriter|
func (c *WebRTCPeer) Read(b []byte) (int, error) {
	return c.recvPipe.Read(b)
}

// Writes bytes out to remote WebRTC.
// As part of |io.ReadWriter|
func (c *WebRTCPeer) Write(b []byte) (int, error) {
	err := c.transport.Send(b)
	if err != nil {
		return 0, err
	}
	c.bytesLogger.addOutbound(len(b))
	return len(b), nil
}

// Closed returns a boolean indicated whether the peer is closed.
func (c *WebRTCPeer) Closed() bool {
	select {
	case <-c.closed:
		return true
	default:
	}
	return false
}

// Close closes the connection the snowflake proxy.
func (c *WebRTCPeer) Close() error {
	c.once.Do(func() {
		close(c.closed)
		c.cleanup()
		log.Printf("WebRTC: Closing")
	})
	return nil
}

// Prevent long-lived broken remotes.
// Should also update the DataChannel in underlying go-webrtc's to make Closes
// more immediate / responsive.
func (c *WebRTCPeer) checkForStaleness(timeout time.Duration) {
	c.mu.Lock()
	c.lastReceive = time.Now()
	c.mu.Unlock()
	for {
		c.mu.Lock()
		lastReceive := c.lastReceive
		c.mu.Unlock()
		if time.Since(lastReceive) > timeout {
			log.Printf("WebRTC: No messages received for %v -- closing stale connection.",
				timeout)
			err := errors.New("no messages received, closing stale connection")
			c.eventsLogger.OnNewSnowflakeEvent(event.EventOnSnowflakeConnectionFailed{Error: err})
			c.Close()
			return
		}
		select {
		case <-c.closed:
			return
		case <-time.After(time.Second):
		}
	}
}

func (c *WebRTCPeer) connect(config *webrtc.Configuration, broker *BrokerChannel) error {
	log.Println(c.id, " connecting...")
	// TODO: When go-webrtc is more stable, it's possible that a new
	// PeerConnection won't need to be re-prepared each time.
	err := c.preparePeerConnection(config)
	localDescription := c.pc.LocalDescription()
	c.eventsLogger.OnNewSnowflakeEvent(event.EventOnOfferCreated{
		WebRTCLocalDescription: localDescription,
		Error:                  err,
	})
	if err != nil {
		return err
	}

	answer, err := broker.Negotiate(localDescription)
	c.eventsLogger.OnNewSnowflakeEvent(event.EventOnBrokerRendezvous{
		WebRTCRemoteDescription: answer,
		Error:                   err,
	})
	if err != nil {
		return err
	}
	log.Printf("Received Answer.\n")
	err = c.pc.SetRemoteDescription(*answer)
	if nil != err {
		log.Println("WebRTC: Unable to SetRemoteDescription:", err)
		return err
	}

	// Wait for the datachannel to open or time out
	select {
	case <-c.open:
	case <-time.After(DataChannelTimeout):
		c.transport.Close()
		err = errors.New("timeout waiting for DataChannel.OnOpen")
		c.eventsLogger.OnNewSnowflakeEvent(event.EventOnSnowflakeConnectionFailed{Error: err})
		return err
	}

	go c.checkForStaleness(SnowflakeTimeout)
	return nil
}

// preparePeerConnection creates a new WebRTC PeerConnection and returns it
// after ICE candidate gathering is complete..
func (c *WebRTCPeer) preparePeerConnection(config *webrtc.Configuration) error {
	var err error
	s := webrtc.SettingEngine{}
	s.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	api := webrtc.NewAPI(webrtc.WithSettingEngine(s))
	c.pc, err = api.NewPeerConnection(*config)
	if err != nil {
		log.Printf("NewPeerConnection ERROR: %s", err)
		return err
	}
	ordered := true
	dataChannelOptions := &webrtc.DataChannelInit{
		Ordered: &ordered,
	}
	// We must create the data channel before creating an offer
	// https://github.com/pion/webrtc/wiki/Release-WebRTC@v3.0.0
	dc, err := c.pc.CreateDataChannel(c.id, dataChannelOptions)
	if err != nil {
		log.Printf("CreateDataChannel ERROR: %s", err)
		return err
	}
	dc.OnOpen(func() {
		c.eventsLogger.OnNewSnowflakeEvent(event.EventOnSnowflakeConnected{})
		log.Println("WebRTC: DataChannel.OnOpen")
		close(c.open)
	})
	dc.OnClose(func() {
		log.Println("WebRTC: DataChannel.OnClose")
		c.Close()
	})
	dc.OnError(func(err error) {
		c.eventsLogger.OnNewSnowflakeEvent(event.EventOnSnowflakeConnectionFailed{Error: err})
	})
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		if len(msg.Data) <= 0 {
			log.Println("0 length message---")
		}
		n, err := c.writePipe.Write(msg.Data)
		c.bytesLogger.addInbound(n)
		if err != nil {
			// TODO: Maybe shouldn't actually close.
			log.Println("Error writing to SOCKS pipe")
			if inerr := c.writePipe.CloseWithError(err); inerr != nil {
				log.Printf("c.writePipe.CloseWithError returned error: %v", inerr)
			}
		}
		c.mu.Lock()
		c.lastReceive = time.Now()
		c.mu.Unlock()
	})
	c.transport = dc
	c.open = make(chan struct{})
	log.Println("WebRTC: DataChannel created.")

	// Allow candidates to accumulate until ICEGatheringStateComplete.
	done := webrtc.GatheringCompletePromise(c.pc)
	offer, err := c.pc.CreateOffer(nil)
	// TODO: Potentially timeout and retry if ICE isn't working.
	if err != nil {
		log.Println("Failed to prepare offer", err)
		c.pc.Close()
		return err
	}
	log.Println("WebRTC: Created offer")
	err = c.pc.SetLocalDescription(offer)
	if err != nil {
		log.Println("Failed to prepare offer", err)
		c.pc.Close()
		return err
	}
	log.Println("WebRTC: Set local description")

	<-done // Wait for ICE candidate gathering to complete.
	log.Println("WebRTC: PeerConnection created.")
	return nil
}

// cleanup closes all channels and transports
func (c *WebRTCPeer) cleanup() {
	// Close this side of the SOCKS pipe.
	if c.writePipe != nil { // c.writePipe can be nil in tests.
		c.writePipe.Close()
	}
	if nil != c.transport {
		log.Printf("WebRTC: closing DataChannel")
		c.transport.Close()
	}
	if nil != c.pc {
		log.Printf("WebRTC: closing PeerConnection")
		err := c.pc.Close()
		if nil != err {
			log.Printf("Error closing peerconnection...")
		}
	}
}
