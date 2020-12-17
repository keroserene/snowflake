package lib

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"sync"
	"time"

	"github.com/pion/webrtc/v3"
)

// Remote WebRTC peer.
//
// Handles preparation of go-webrtc PeerConnection. Only ever has
// one DataChannel.
type WebRTCPeer struct {
	id        string
	pc        *webrtc.PeerConnection
	transport *webrtc.DataChannel

	recvPipe    *io.PipeReader
	writePipe   *io.PipeWriter
	lastReceive time.Time

	open   chan struct{} // Channel to notify when datachannel opens
	closed bool

	once sync.Once // Synchronization for PeerConnection destruction

	BytesLogger BytesLogger
}

// Construct a WebRTC PeerConnection.
func NewWebRTCPeer(config *webrtc.Configuration,
	broker *BrokerChannel) (*WebRTCPeer, error) {
	connection := new(WebRTCPeer)
	{
		var buf [8]byte
		if _, err := rand.Read(buf[:]); err != nil {
			panic(err)
		}
		connection.id = "snowflake-" + hex.EncodeToString(buf[:])
	}

	// Override with something that's not NullLogger to have real logging.
	connection.BytesLogger = &BytesNullLogger{}

	// Pipes remain the same even when DataChannel gets switched.
	connection.recvPipe, connection.writePipe = io.Pipe()

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
	c.BytesLogger.AddOutbound(len(b))
	return len(b), nil
}

func (c *WebRTCPeer) Close() error {
	c.once.Do(func() {
		c.closed = true
		c.cleanup()
		log.Printf("WebRTC: Closing")
	})
	return nil
}

// Prevent long-lived broken remotes.
// Should also update the DataChannel in underlying go-webrtc's to make Closes
// more immediate / responsive.
func (c *WebRTCPeer) checkForStaleness() {
	c.lastReceive = time.Now()
	for {
		if c.closed {
			return
		}
		if time.Since(c.lastReceive) > SnowflakeTimeout {
			log.Printf("WebRTC: No messages received for %v -- closing stale connection.",
				SnowflakeTimeout)
			c.Close()
			return
		}
		<-time.After(time.Second)
	}
}

func (c *WebRTCPeer) connect(config *webrtc.Configuration, broker *BrokerChannel) error {
	log.Println(c.id, " connecting...")
	// TODO: When go-webrtc is more stable, it's possible that a new
	// PeerConnection won't need to be re-prepared each time.
	c.preparePeerConnection(config)
	answer, err := broker.Negotiate(c.pc.LocalDescription())
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
		return errors.New("timeout waiting for DataChannel.OnOpen")
	}

	go c.checkForStaleness()
	return nil
}

// preparePeerConnection creates a new WebRTC PeerConnection and returns it
// after ICE candidate gathering is complete..
func (c *WebRTCPeer) preparePeerConnection(config *webrtc.Configuration) error {
	var err error
	c.pc, err = webrtc.NewPeerConnection(*config)
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
		log.Println("WebRTC: DataChannel.OnOpen")
		close(c.open)
	})
	dc.OnClose(func() {
		log.Println("WebRTC: DataChannel.OnClose")
		c.Close()
	})
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		if len(msg.Data) <= 0 {
			log.Println("0 length message---")
		}
		n, err := c.writePipe.Write(msg.Data)
		c.BytesLogger.AddInbound(n)
		if err != nil {
			// TODO: Maybe shouldn't actually close.
			log.Println("Error writing to SOCKS pipe")
			if inerr := c.writePipe.CloseWithError(err); inerr != nil {
				log.Printf("c.writePipe.CloseWithError returned error: %v", inerr)
			}
		}
		c.lastReceive = time.Now()
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

// Close all channels and transports
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
