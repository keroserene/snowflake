package lib

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"sync"
	"time"

	"github.com/pion/webrtc/v2"
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
	var err error
	c.pc, err = preparePeerConnection(config)
	if err != nil {
		return err
	}
	answer := exchangeSDP(broker, c.pc.LocalDescription())
	log.Printf("Received Answer.\n")
	err = c.pc.SetRemoteDescription(*answer)
	if nil != err {
		log.Println("WebRTC: Unable to SetRemoteDescription:", err)
		return err
	}
	c.transport, err = c.establishDataChannel()
	if err != nil {
		log.Printf("establishDataChannel: %v", err)
		// nolint: golint
		return errors.New("WebRTC: Could not establish DataChannel")
	}
	go c.checkForStaleness()
	return nil
}

// preparePeerConnection creates a new WebRTC PeerConnection and returns it
// after ICE candidate gathering is complete..
func preparePeerConnection(config *webrtc.Configuration) (*webrtc.PeerConnection, error) {
	pc, err := webrtc.NewPeerConnection(*config)
	if err != nil {
		log.Printf("NewPeerConnection ERROR: %s", err)
		return nil, err
	}
	// Prepare PeerConnection callbacks.
	offerChannel := make(chan struct{})
	// Allow candidates to accumulate until ICEGatheringStateComplete.
	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			log.Printf("WebRTC: Done gathering candidates")
			close(offerChannel)
		} else {
			log.Printf("WebRTC: Got ICE candidate: %s", candidate.String())
		}
	})

	offer, err := pc.CreateOffer(nil)
	// TODO: Potentially timeout and retry if ICE isn't working.
	if err != nil {
		log.Println("Failed to prepare offer", err)
		pc.Close()
		return nil, err
	}
	log.Println("WebRTC: Created offer")
	err = pc.SetLocalDescription(offer)
	if err != nil {
		log.Println("Failed to prepare offer", err)
		pc.Close()
		return nil, err
	}
	log.Println("WebRTC: Set local description")

	<-offerChannel // Wait for ICE candidate gathering to complete.
	log.Println("WebRTC: PeerConnection created.")
	return pc, nil
}

// Create a WebRTC DataChannel locally. Blocks until the data channel is open,
// or a timeout or error occurs.
func (c *WebRTCPeer) establishDataChannel() (*webrtc.DataChannel, error) {
	ordered := true
	dataChannelOptions := &webrtc.DataChannelInit{
		Ordered: &ordered,
	}
	dc, err := c.pc.CreateDataChannel(c.id, dataChannelOptions)
	if err != nil {
		log.Printf("CreateDataChannel ERROR: %s", err)
		return nil, err
	}
	openChannel := make(chan struct{})
	dc.OnOpen(func() {
		log.Println("WebRTC: DataChannel.OnOpen")
		close(openChannel)
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
	log.Println("WebRTC: DataChannel created.")

	select {
	case <-openChannel:
		return dc, nil
	case <-time.After(DataChannelTimeout):
		dc.Close()
		return nil, errors.New("timeout waiting for DataChannel.OnOpen")
	}
}

// exchangeSDP sends the local SDP offer to the Broker, awaits the SDP answer,
// and returns the answer.
func exchangeSDP(broker *BrokerChannel, offer *webrtc.SessionDescription) *webrtc.SessionDescription {
	// Keep trying the same offer until a valid answer arrives.
	for {
		// Send offer to broker (blocks).
		answer, err := broker.Negotiate(offer)
		if err == nil {
			return answer
		}
		log.Printf("BrokerChannel Error: %s", err)
		log.Printf("Failed to retrieve answer. Retrying in %v", ReconnectTimeout)
		<-time.After(ReconnectTimeout)
	}
}

// Close all channels and transports
func (c *WebRTCPeer) cleanup() {
	// Close this side of the SOCKS pipe.
	c.writePipe.Close()
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
