package main

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/keroserene/go-webrtc"
	"io"
	"log"
	"net"
	"time"
)

// Implements net.Conn interface
type webRTCConn struct {
	config        *webrtc.Configuration
	pc            *webrtc.PeerConnection
	snowflake     SnowflakeChannel // Interface holding the WebRTC DataChannel.
	broker        *BrokerChannel
	offerChannel  chan *webrtc.SessionDescription
	answerChannel chan *webrtc.SessionDescription
	errorChannel  chan error
	recvPipe      *io.PipeReader
	writePipe     *io.PipeWriter
	buffer        bytes.Buffer
	reset         chan struct{}
	*BytesInfo
}

var webrtcRemote *webRTCConn

func (c *webRTCConn) Read(b []byte) (int, error) {
	return c.recvPipe.Read(b)
}

// Writes bytes out to the snowflake proxy.
func (c *webRTCConn) Write(b []byte) (int, error) {
	c.BytesInfo.AddOutbound(len(b))
	if nil == c.snowflake {
		log.Printf("Buffered %d bytes --> WebRTC", len(b))
		c.buffer.Write(b)
	} else {
		c.snowflake.Send(b)
	}
	return len(b), nil
}

func (c *webRTCConn) Close() error {
	var err error = nil
	log.Printf("WebRTC: Closing")
	c.cleanup()
	close(c.offerChannel)
	close(c.answerChannel)
	close(c.errorChannel)
	return err
}

func (c *webRTCConn) LocalAddr() net.Addr {
	return nil
}

func (c *webRTCConn) RemoteAddr() net.Addr {
	return nil
}

func (c *webRTCConn) SetDeadline(t time.Time) error {
	return fmt.Errorf("SetDeadline not implemented")
}

func (c *webRTCConn) SetReadDeadline(t time.Time) error {
	return fmt.Errorf("SetReadDeadline not implemented")
}

func (c *webRTCConn) SetWriteDeadline(t time.Time) error {
	return fmt.Errorf("SetWriteDeadline not implemented")
}

func NewWebRTCConnection(config *webrtc.Configuration,
	broker *BrokerChannel) *webRTCConn {
	connection := new(webRTCConn)
	connection.config = config
	connection.broker = broker
	connection.offerChannel = make(chan *webrtc.SessionDescription, 1)
	connection.answerChannel = make(chan *webrtc.SessionDescription, 1)
	// Error channel is mostly for reporting during the initial SDP offer
	// creation & local description setting, which happens asynchronously.
	connection.errorChannel = make(chan error, 1)
	connection.reset = make(chan struct{}, 1)

	// Log every few seconds.
	connection.BytesInfo = &BytesInfo{
		inboundChan: make(chan int, 5), outboundChan: make(chan int, 5),
		inbound: 0, outbound: 0, inEvents: 0, outEvents: 0,
	}
	go connection.BytesInfo.Log()

	// Pipes remain the same even when DataChannel gets switched.
	connection.recvPipe, connection.writePipe = io.Pipe()

	return connection
}

// TODO: Multiplex.
func (c *webRTCConn) Connect() error {
	log.Println("Establishing WebRTC connection...")
	// TODO: When go-webrtc is more stable, it's possible that a new
	// PeerConnection won't need to be re-prepared each time.
	err := c.preparePeerConnection()
	if err != nil {
		return err
	}
	err = c.establishDataChannel()
	if err != nil {
		return errors.New("WebRTC: Could not establish DataChannel.")
	}
	err = c.exchangeSDP()
	if err != nil {
		return err
	}
	return nil
}

// Create and prepare callbacks on a new WebRTC PeerConnection.
func (c *webRTCConn) preparePeerConnection() error {
	if nil != c.pc {
		c.pc.Close()
		c.pc = nil
	}
	pc, err := webrtc.NewPeerConnection(c.config)
	if err != nil {
		log.Printf("NewPeerConnection ERROR: %s", err)
		return err
	}
	// Prepare PeerConnection callbacks.
	pc.OnNegotiationNeeded = func() {
		log.Println("WebRTC: OnNegotiationNeeded")
		go func() {
			offer, err := pc.CreateOffer()
			// TODO: Potentially timeout and retry if ICE isn't working.
			if err != nil {
				c.errorChannel <- err
				return
			}
			err = pc.SetLocalDescription(offer)
			if err != nil {
				c.errorChannel <- err
				return
			}
		}()
	}
	// Allow candidates to accumulate until OnIceComplete.
	pc.OnIceCandidate = func(candidate webrtc.IceCandidate) {
		log.Printf(candidate.Candidate)
	}
	// TODO: This may soon be deprecated, consider OnIceGatheringStateChange.
	pc.OnIceComplete = func() {
		log.Printf("WebRTC: OnIceComplete")
		c.offerChannel <- pc.LocalDescription()
	}
	// This callback is not expected, as the Client initiates the creation
	// of the data channel, not the remote peer.
	pc.OnDataChannel = func(channel *webrtc.DataChannel) {
		log.Println("OnDataChannel")
		panic("Unexpected OnDataChannel!")
	}
	c.pc = pc
	log.Println("WebRTC: PeerConnection created.")
	return nil
}

// Create a WebRTC DataChannel locally.
func (c *webRTCConn) establishDataChannel() error {
	if c.snowflake != nil {
		panic("Unexpected datachannel already exists!")
	}
	dc, err := c.pc.CreateDataChannel("snowflake", webrtc.Init{})
	// Triggers "OnNegotiationNeeded" on the PeerConnection, which will prepare
	// an SDP offer while other goroutines operating on this struct handle the
	// signaling. Eventually fires "OnOpen".
	if err != nil {
		log.Printf("CreateDataChannel ERROR: %s", err)
		return err
	}
	dc.OnOpen = func() {
		log.Println("WebRTC: DataChannel.OnOpen")
		if nil != c.snowflake {
			log.Println("PeerConnection snowflake already exists.")
			panic("PeerConnection snowflake already exists.")
		}
		// Flush buffered outgoing SOCKS data if necessary.
		if c.buffer.Len() > 0 {
			dc.Send(c.buffer.Bytes())
			log.Println("Flushed", c.buffer.Len(), "bytes.")
			c.buffer.Reset()
		}
		// Then enable the datachannel.
		c.snowflake = dc
	}
	dc.OnClose = func() {
		// Future writes will go to the buffer until a new DataChannel is available.
		if nil == c.snowflake {
			// Closed locally, as part of a reset.
			log.Println("WebRTC: DataChannel.OnClose [locally]")
			return
		}
		// Closed remotely, need to reset everything.
		// Disable the DataChannel as a write destination.
		log.Println("WebRTC: DataChannel.OnClose [remotely]")
		c.snowflake = nil
		// TODO(issue #12): Need a way to update the circuit so that when a new WebRTC
		// data channel is available, the relay actually recognizes the new
		// snowflake.
		c.Reset()
	}
	dc.OnMessage = func(msg []byte) {
		if len(msg) <= 0 {
			log.Println("0 length message---")
		}
		c.BytesInfo.AddInbound(len(msg))
		n, err := c.writePipe.Write(msg)
		if err != nil {
			// TODO: Maybe shouldn't actually close.
			log.Println("Error writing to SOCKS pipe")
			c.writePipe.CloseWithError(err)
		}
		if n != len(msg) {
			log.Println("Error: short write")
			panic("short write")
		}
	}
	log.Println("WebRTC: DataChannel created.")
	return nil
}

func (c *webRTCConn) sendOfferToBroker() {
	if "" == brokerURL {
		return
	}
	offer := c.pc.LocalDescription()
	log.Println("Sending offer via BrokerChannel...\nTarget URL: ", brokerURL,
		"\nFront URL:  ", frontDomain)
	answer, err := c.broker.Negotiate(offer)
	if nil != err || nil == answer {
		log.Printf("BrokerChannel Error: %s", err)
		answer = nil
	}
	c.answerChannel <- answer
}

// Block until an SDP offer is available, send it to either
// the Broker or signal pipe, then await for the SDP answer.
func (c *webRTCConn) exchangeSDP() error {
	select {
	case offer := <-c.offerChannel:
		// Display for copy-paste, when no broker available.
		if "" == brokerURL {
			log.Printf("Please Copy & Paste the following to the peer:")
			log.Printf("----------------")
			log.Printf("\n" + offer.Serialize() + "\n")
			log.Printf("----------------")
		}
	case err := <-c.errorChannel:
		log.Println("Failed to prepare offer", err)
		c.Reset()
		return err
	}
	// Keep trying the same offer until a valid answer arrives.
	var ok bool
	var answer *webrtc.SessionDescription = nil
	for nil == answer {
		go c.sendOfferToBroker()
		answer, ok = <-c.answerChannel // Blocks...
		if !ok || nil == answer {
			log.Printf("Failed to retrieve answer. Retrying in %d seconds", ReconnectTimeout)
			<-time.After(time.Second * ReconnectTimeout)
			answer = nil
		}
	}
	log.Printf("Received Answer:\n\n%s\n", answer.Sdp)
	err := c.pc.SetRemoteDescription(answer)
	if nil != err {
		log.Println("WebRTC: Unable to SetRemoteDescription:", err)
		return err
	}
	return nil
}

func (c *webRTCConn) Reset() {
	go func() {
		c.reset <- struct{}{}
		log.Println("WebRTC resetting...")
	}()
}

func (c *webRTCConn) cleanup() {
	if nil != c.snowflake {
		s := c.snowflake
		log.Printf("WebRTC: closing DataChannel")
		// Setting snowflak to nil *before* Close indicates to OnClose that it
		// was locally triggered.
		c.snowflake = nil
		s.Close()
	}
	if nil != c.pc {
		log.Printf("WebRTC: closing PeerConnection")
		err := c.pc.Close()
		if nil != err {
			log.Printf("Error closing peerconnection...")
		}
		c.pc = nil
	}
}
