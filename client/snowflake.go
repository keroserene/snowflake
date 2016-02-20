// Client transport plugin for the Snowflake pluggable transport.
package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"git.torproject.org/pluggable-transports/goptlib.git"
	"github.com/keroserene/go-webrtc"
)

var ptInfo pt.ClientInfo
var logFile *os.File
var brokerURL string
var frontDomain string

// When a connection handler starts, +1 is written to this channel; when it
// ends, -1 is written.
var handlerChan = make(chan int)

const (
	ReconnectTimeout = 5
)

func copyLoop(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	// TODO fix the copy loop.
	go func() {
		io.Copy(b, a)
		log.Println("copy loop b-a break")
		wg.Done()
	}()
	go func() {
		io.Copy(a, b)
		log.Println("copy loop a-b break")
		wg.Done()
	}()
	wg.Wait()
	log.Println("copy loop ended")
}

// Interface that matches both webrc.DataChannel and for testing.
type SnowflakeChannel interface {
	Send([]byte)
	Close() error
}

// Implements net.Conn interface
type webRTCConn struct {
	config        *webrtc.Configuration
	pc            *webrtc.PeerConnection
	snowflake     SnowflakeChannel // Interface holding the WebRTC DataChannel.
	broker        *BrokerChannel
	offerChannel  chan *webrtc.SessionDescription
	answerChannel chan *webrtc.SessionDescription
	errorChannel  chan error
	writeChannel  chan []byte
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

func (c *webRTCConn) Write(b []byte) (int, error) {
	c.SendData(b)
	return len(b), nil
}

func (c *webRTCConn) Close() error {
	// Data channel closed implicitly?
	return c.pc.Close()
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

func (c *webRTCConn) PreparePeerConnection() {
	if nil != c.pc {
		c.pc.Close()
		c.pc = nil
	}
	pc, err := webrtc.NewPeerConnection(c.config)
	if err != nil {
		log.Printf("NewPeerConnection: %s", err)
		c.errorChannel <- err
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
}

// Create a WebRTC DataChannel locally.
func (c *webRTCConn) EstablishDataChannel() error {
	dc, err := c.pc.CreateDataChannel("snowflake", webrtc.Init{})
	// Triggers "OnNegotiationNeeded" on the PeerConnection, which will prepare
	// an SDP offer while other goroutines operating on this struct handle the
	// signaling. Eventually fires "OnOpen".
	if err != nil {
		log.Printf("CreateDataChannel: %s", err)
		return err
	}
	dc.OnOpen = func() {
		log.Println("WebRTC: DataChannel.OnOpen")
		// if nil != c.snowflake {
		// panic("PeerConnection snowflake already exists.")
		// }
		// Flush the buffer, then enable datachannel.
		// TODO: Make this more safe
		// dc.Send(c.buffer.Bytes())
		// log.Println("Flushed", c.buffer.Len(), "bytes")
		// c.buffer.Reset()
		c.snowflake = dc
		c.SendData(nil)
	}
	dc.OnClose = func() {
		// Disable the DataChannel as a write destination.
		// Future writes will go to the buffer until a new DataChannel is available.
		log.Println("WebRTC: DataChannel.OnClose")
		if nil != c.snowflake {
			c.snowflake = nil
			// Only reset if this OnClose was triggered remotely.
			c.Reset()
		}
	}
	dc.OnMessage = func(msg []byte) {
		c.BytesInfo.AddInbound(len(msg))
		n, err := c.writePipe.Write(msg)
		if err != nil {
			// TODO: Maybe shouldn't actually close.
			log.Println("Error writing to SOCKS pipe")
			c.writePipe.CloseWithError(err)
		}
		if n != len(msg) {
			panic("short write")
		}
	}
	log.Println("WebRTC: DataChannel created.")
	return nil
}

// Block until an offer is available, then send it to either
// the Broker or signal pipe.
func (c *webRTCConn) SendOffer() error {
	select {
	case offer := <-c.offerChannel:
		if "" == brokerURL {
			log.Printf("Please Copy & Paste the following to the peer:")
			log.Printf("----------------")
			fmt.Fprintln(logFile, "\n"+offer.Serialize()+"\n")
			log.Printf("----------------")
			return nil
		}
		// Otherwise, use Broker.
		go func() {
			log.Println("Sending offer via BrokerChannel...\nTarget URL: ", brokerURL,
				"\nFront URL:  ", frontDomain)
			answer, err := c.broker.Negotiate(c.pc.LocalDescription())
			if nil != err || nil == answer {
				log.Printf("BrokerChannel error: %s", err)
				answer = nil
			}
			c.answerChannel <- answer
		}()
	case err := <-c.errorChannel:
		c.pc.Close()
		return err
	}
	return nil
}

func (c *webRTCConn) ReceiveAnswer() {
	go func() {
		answer, ok := <-c.answerChannel
		if !ok || nil == answer {
			log.Printf("Failed to retrieve answer. Retrying in %d seconds", ReconnectTimeout)
			<-time.After(time.Second * ReconnectTimeout)
			c.Reset()
			return
		}
		log.Printf("Received Answer:\n\n%s\n", answer.Sdp)
		err := c.pc.SetRemoteDescription(answer)
		if nil != err {
			c.errorChannel <- err
		}
	}()
}

func (c *webRTCConn) SendData(data []byte) {
	// Buffer the data in case datachannel isn't available yet.
	if nil == c.snowflake {
		log.Printf("Buffered %d bytes --> WebRTC", len(data))
		c.buffer.Write(data)
		return
	}
	go func() {
		c.writeChannel <- data
	}()
}

// Expected in own goroutine.
func (c *webRTCConn) SendLoop() {
	log.Println("send loop")
	for data := range c.writeChannel {
		// Flush buffer if necessary.
		for c.buffer.Len() > 0 {
			c.snowflake.Send(c.buffer.Bytes())
			log.Println("Flushed", c.buffer.Len(), "bytes")
			c.buffer.Reset()
		}

		c.BytesInfo.AddOutbound(len(data))
		c.snowflake.Send(data)
	}
}

// WebRTC re-establishment loop. Expected in own goroutine.
func (c *webRTCConn) ConnectLoop() {
	for {
		log.Println("Establishing WebRTC connection...")
		// TODO: When go-webrtc is more stable, it's possible that a new
		// PeerConnection won't need to be re-prepared each time.
		c.PreparePeerConnection()
		err := c.EstablishDataChannel()
		if err == nil {
			c.SendOffer()
			c.ReceiveAnswer()
			<-c.reset
			log.Println(" --- snowflake connection reset ---")
		}
	}
}

func (c *webRTCConn) Reset() {
	go func() {
		c.reset <- struct{}{} // Attempt to negotiate a new datachannel..
		log.Println("WebRTC resetting...")
	}()
}

// Initialize a WebRTC Connection.
func dialWebRTC(config *webrtc.Configuration, broker *BrokerChannel) (
	*webRTCConn, error) {
	connection := new(webRTCConn)
	connection.config = config
	connection.broker = broker
	connection.offerChannel = make(chan *webrtc.SessionDescription)
	connection.answerChannel = make(chan *webrtc.SessionDescription)
	connection.writeChannel = make(chan []byte)
	connection.errorChannel = make(chan error)
	connection.reset = make(chan struct{})
	connection.BytesInfo = &BytesInfo{
		inboundChan: make(chan int), outboundChan: make(chan int),
		inbound: 0, outbound: 0, inEvents: 0, outEvents: 0,
	}
	go connection.BytesInfo.Log()

	// Pipes remain the same even when DataChannel gets switched.
	connection.recvPipe, connection.writePipe = io.Pipe()

	go connection.ConnectLoop()
	go connection.SendLoop()
	return connection, nil
}

func endWebRTC() {
	log.Printf("WebRTC: interruped")
	if nil == webrtcRemote {
		return
	}
	if nil != webrtcRemote.snowflake {
		s := webrtcRemote.snowflake
		webrtcRemote.snowflake = nil
		log.Printf("WebRTC: closing DataChannel")
		s.Close()
	}
	if nil != webrtcRemote.pc {
		log.Printf("WebRTC: closing PeerConnection")
		webrtcRemote.pc.Close()
	}
}

// Establish a WebRTC channel for SOCKS connections.
func handler(conn *pt.SocksConn) error {
	handlerChan <- 1
	defer func() {
		handlerChan <- -1
	}()
	defer conn.Close()
	log.Println("handler", conn)

	// TODO: [#3] Fetch ICE server information from Broker.
	// TODO: [#18] Consider TURN servers here too.
	config := webrtc.NewConfiguration(
		webrtc.OptionIceServer("stun:stun.l.google.com:19302"))
	broker := NewBrokerChannel(brokerURL, frontDomain)
	if nil == broker {
		conn.Reject()
		return errors.New("Failed to prepare BrokerChannel")
	}
	remote, err := dialWebRTC(config, broker)
	if err != nil {
		conn.Reject()
		return err
	}
	defer remote.Close()
	webrtcRemote = remote

	err = conn.Grant(&net.TCPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return err
	}

	copyLoop(conn, remote)
	log.Println("----END---")
	return nil
}

func acceptLoop(ln *pt.SocksListener) error {
	defer ln.Close()
	for {
		conn, err := ln.AcceptSocks()
		if err != nil {
			if e, ok := err.(net.Error); ok && e.Temporary() {
				continue
			}
			return err
		}
		go func() {
			err := handler(conn)
			if err != nil {
				log.Printf("handler error: %s", err)
			}
		}()
	}
}

func readSignalingMessages(f *os.File) {
	log.Printf("readSignalingMessages")
	s := bufio.NewScanner(f)
	for s.Scan() {
		msg := s.Text()
		log.Printf("readSignalingMessages loop %+q", msg)
		sdp := webrtc.DeserializeSessionDescription(msg)
		if sdp == nil {
			log.Printf("ignoring invalid signal message %+q", msg)
			continue
		}
		webrtcRemote.answerChannel <- sdp
	}
	log.Printf("close answerChannel")
	close(webrtcRemote.answerChannel)
	if err := s.Err(); err != nil {
		log.Printf("signal FIFO: %s", err)
	}
}

func main() {
	var err error
	webrtc.SetLoggingVerbosity(1)
	flag.StringVar(&brokerURL, "url", "", "URL of signaling broker")
	flag.StringVar(&frontDomain, "front", "", "front domain")
	flag.Parse()
	logFile, err = os.OpenFile("snowflake.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		log.Fatal(err)
	}
	defer logFile.Close()
	log.SetOutput(logFile)
	log.Println("\nStarting Snowflake Client...")

	// Expect user to copy-paste if
	// TODO: Maybe just get rid of copy-paste entirely.
	if "" != brokerURL {
		log.Println("Rendezvous using Broker at: ", brokerURL)
		if "" != frontDomain {
			log.Println("Domain fronting using:", frontDomain)
		}
	} else {
		log.Println("No HTTP signaling detected. Waiting for a \"signal\" pipe...")
		// This FIFO receives signaling messages.
		err := syscall.Mkfifo("signal", 0600)
		if err != nil {
			if err.(syscall.Errno) != syscall.EEXIST {
				log.Fatal(err)
			}
		}
		signalFile, err := os.OpenFile("signal", os.O_RDONLY, 0600)
		if err != nil {
			log.Fatal(err)
		}
		defer signalFile.Close()
		go readSignalingMessages(signalFile)
	}

	ptInfo, err = pt.ClientSetup(nil)
	if err != nil {
		log.Fatal(err)
	}

	if ptInfo.ProxyURL != nil {
		pt.ProxyError("proxy is not supported")
		os.Exit(1)
	}

	listeners := make([]net.Listener, 0)
	for _, methodName := range ptInfo.MethodNames {
		switch methodName {
		case "snowflake":
			ln, err := pt.ListenSocks("tcp", "127.0.0.1:0")
			if err != nil {
				pt.CmethodError(methodName, err.Error())
				break
			}
			go acceptLoop(ln)
			pt.Cmethod(methodName, ln.Version(), ln.Addr())
			listeners = append(listeners, ln)
		default:
			pt.CmethodError(methodName, "no such method")
		}
	}
	pt.CmethodsDone()
	defer endWebRTC()

	var numHandlers int = 0
	var sig os.Signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// wait for first signal
	sig = nil
	for sig == nil {
		select {
		case n := <-handlerChan:
			numHandlers += n
		case sig = <-sigChan:
		}
	}
	for _, ln := range listeners {
		ln.Close()
	}

	// wait for second signal or no more handlers
	sig = nil
	for sig == nil && numHandlers != 0 {
		select {
		case n := <-handlerChan:
			numHandlers += n
		case sig = <-sigChan:
		}
	}
}
