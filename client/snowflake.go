// Client transport plugin for the Snowflake pluggable transport.
package main

import (
	"bufio"
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
	"github.com/keroserene/go-webrtc/data"
)

var ptInfo pt.ClientInfo
var logFile *os.File
var brokerURL string
var frontDomain string

// When a connection handler starts, +1 is written to this channel; when it
// ends, -1 is written.
var handlerChan = make(chan int)

var signalChan = make(chan *webrtc.SessionDescription)

func copyLoop(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		io.Copy(b, a)
		wg.Done()
	}()
	go func() {
		io.Copy(a, b)
		wg.Done()
	}()

	wg.Wait()
}

type webRTCConn struct {
	pc       *webrtc.PeerConnection
	dc       *data.Channel
	recvPipe *io.PipeReader
}

var webrtcRemote *webRTCConn

func (c *webRTCConn) Read(b []byte) (int, error) {
	return c.recvPipe.Read(b)
}

func (c *webRTCConn) Write(b []byte) (int, error) {
	// log.Printf("webrtc Write %d %+q", len(b), string(b))
	log.Printf("Write %d bytes --> WebRTC", len(b))
	c.dc.Send(b)
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

func dialWebRTC(config *webrtc.Configuration, broker *BrokerChannel) (
	*webRTCConn, error) {

	offerChan := make(chan *webrtc.SessionDescription)
	errChan := make(chan error)
	openChan := make(chan struct{})

	pc, err := webrtc.NewPeerConnection(config)
	if err != nil {
		log.Printf("NewPeerConnection: %s", err)
		return nil, err
	}

	// Triggered by CreateDataChannel.
	pc.OnNegotiationNeeded = func() {
		log.Println("OnNegotiationNeeded")
		go func() {
			offer, err := pc.CreateOffer()
			if err != nil {
				errChan <- err
				return
			}
			err = pc.SetLocalDescription(offer)
			if err != nil {
				errChan <- err
				return
			}
		}()
	}
	pc.OnIceCandidate = func(candidate webrtc.IceCandidate) {
		log.Printf("OnIceCandidate %s", candidate.Serialize())
		// Allow candidates to accumulate until OnIceComplete.
	}
	// TODO: This may soon be deprecated, consider OnIceGatheringStateChange.
	pc.OnIceComplete = func() {
		log.Printf("OnIceComplete")
		offerChan <- pc.LocalDescription()
	}
	// This callback is not expected, as the Client initiates the creation
	// of the data channel, not the remote peer.
	pc.OnDataChannel = func(channel *data.Channel) {
		log.Println("OnDataChannel")
		panic("OnDataChannel")
	}

	pr, pw := io.Pipe()

	dc, err := pc.CreateDataChannel("test", data.Init{})
	if err != nil {
		log.Printf("CreateDataChannel: %s", err)
		return nil, err
	}
	dc.OnOpen = func() {
		log.Println("OnOpen channel")
		openChan <- struct{}{}
	}
	dc.OnClose = func() {
		log.Println("OnClose channel")
		pw.Close()
		close(openChan)
		// TODO: (Issue #12) Should attempt to renegotiate at this point.
	}
	dc.OnMessage = func(msg []byte) {
		log.Printf("OnMessage <--- %d bytes", len(msg))
		n, err := pw.Write(msg)
		if err != nil {
			pw.CloseWithError(err)
		}
		if n != len(msg) {
			panic("short write")
		}
	}

	select {
	case err := <-errChan:
		pc.Close()
		return nil, err
	case offer := <-offerChan:
		log.Printf("----------------")
		fmt.Fprintln(logFile, "\n"+offer.Serialize()+"\n")
		log.Printf("----------------")
		go func() {
			if "" != brokerURL {
				log.Println("Sending offer via BrokerChannel...\nTarget URL: ", brokerURL,
					"\nFront URL:  ", frontDomain)
				answer, err := broker.Negotiate(pc.LocalDescription())
				if nil != err {
					log.Printf("BrokerChannel signaling error: %s", err)
				}
				if nil == answer {
					log.Printf("BrokerChannel: No answer received.")
				} else {
					signalChan <- answer
				}
			}
		}()
	}

	log.Printf("waiting for answer")
	answer, ok := <-signalChan

	if !ok {
		pc.Close()
		return nil, fmt.Errorf("no answer received")
	}
	log.Printf("Received Answer:\n\n%s\n", answer.Sdp)
	err = pc.SetRemoteDescription(answer)
	if err != nil {
		pc.Close()
		return nil, err
	}

	// Wait until data channel is open; otherwise for example sends may get
	// lost.
	// TODO: Buffering *should* work though.
	_, ok = <-openChan
	if !ok {
		pc.Close()
		return nil, fmt.Errorf("failed to open data channel")
	}

	return &webRTCConn{pc: pc, dc: dc, recvPipe: pr}, nil
}

func endWebRTC() {
	log.Printf("WebRTC: interruped")
	if nil == webrtcRemote {
		return
	}
	if nil != webrtcRemote.dc {
		log.Printf("WebRTC: closing DataChannel")
		webrtcRemote.dc.Close()
	}
	if nil != webrtcRemote.pc {
		log.Printf("WebRTC: closing PeerConnection")
		webrtcRemote.pc.Close()
	}
}

func handler(conn *pt.SocksConn) error {
	handlerChan <- 1
	defer func() {
		handlerChan <- -1
	}()
	defer conn.Close()

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
		signalChan <- sdp
	}
	log.Printf("close signalChan")
	close(signalChan)
	if err := s.Err(); err != nil {
		log.Printf("signal FIFO: %s", err)
	}
}

func main() {
	var err error

	flag.StringVar(&brokerURL, "url", "", "URL of signaling broker")
	flag.StringVar(&frontDomain, "front", "", "front domain")
	flag.Parse()

	logFile, err = os.OpenFile("snowflake.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		log.Fatal(err)
	}
	defer logFile.Close()
	log.SetOutput(logFile)
	log.Println("starting")

	if "" == brokerURL {
		log.Println("No HTTP signaling detected. Waiting for a \"signal\" pipe...")
		// This FIFO receives signaling messages.
		err = syscall.Mkfifo("signal", 0600)
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

	webrtc.SetLoggingVerbosity(1)

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

	if syscall.SIGTERM == sig || syscall.SIGINT == sig {
		endWebRTC()
		return
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
