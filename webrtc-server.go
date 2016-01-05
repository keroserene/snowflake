package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/keroserene/go-webrtc"
	"github.com/keroserene/go-webrtc/data"

	"git.torproject.org/pluggable-transports/goptlib.git"
)

var ptInfo pt.ServerInfo
var logFile *os.File

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

func (c *webRTCConn) Read(b []byte) (int, error) {
	return c.recvPipe.Read(b)
}

func (c *webRTCConn) Write(b []byte) (int, error) {
	log.Printf("webrtc Write %d %q", len(b), string(b))
	err := c.dc.Send(b)
	if err != nil {
		return 0, err
	}
	return len(b), err
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

type webRTCListener struct {
	peerConnectionChan chan *webrtc.PeerConnection
	stopChan           chan struct{}
}

func (ln *webRTCListener) Accept() (net.Conn, error) {
	offer, ok := <-signalChan
	if !ok {
		return nil, fmt.Errorf("signal channel closed")
	}

	pc, ok := <-ln.peerConnectionChan
	if !ok {
		return nil, fmt.Errorf("PeerConnection channel closed")
	}

	err := pc.SetRemoteDescription(offer)
	if err != nil {
		return nil, err
	}

	go func() {
		answer, err := pc.CreateAnswer()
		if err != nil {
			// signal error upwards
			fmt.Println(err)
			return
		}
		err = pc.SetLocalDescription(answer)
		if err != nil {
			// signal error upwards
			fmt.Println(err)
			return
		}
	}()

	select {
	case conn := <-ln.connChan:
		return conn, nil
	case err := <-ln.errChan:
		return nil, err
	}

	return &webRTCConn{pc: pc, dc: dc, recvPipe: nil}, nil
}

func (ln *webRTCListener) Close() error {
	// Stop the PeerConnection factory goroutine.
	close(ln.stopChan)
	return nil
}

func (ln *webRTCListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4zero, Port: 1}
}

func makePeerConnection(config *webrtc.Configuration) (*webrtc.PeerConnection, error) {
	pc, err := webrtc.NewPeerConnection(config)
	if err != nil {
		log.Printf("NewPeerConnection: %s", err)
		return nil, err
	}

	pc.OnNegotiationNeeded = func() {
		log.Println("OnNegotiationNeeded")
		panic("OnNegotiationNeeded")
	}
	pc.OnIceCandidate = func(candidate webrtc.IceCandidate) {
		log.Printf("OnIceCandidate %s", candidate.Serialize())
		// Allow candidates to accumulate until OnIceComplete.
	}
	pc.OnIceComplete = func() {
		log.Printf("OnIceComplete")
	}
	pc.OnDataChannel = func(channel *data.Channel) {
		log.Println("OnDataChannel")
		panic("OnDataChannel")
	}

	return pc, nil
}

func listenWebRTC(config *webrtc.Configuration) (*webRTCListener, error) {
	ln := new(webRTCListener)
	ln.peerConnectionChan = make(chan *webrtc.PeerConnection)
	ln.stopChan = make(chan struct{})

	// This goroutine builds new PeerConnections that await incoming offers.
	go func() {
	loop:
		for {
			select {
			case <-ln.stopChan:
				break loop
			default:
				pc, err := makePeerConnection(config)
				if err != nil {
					log.Printf("makePeerConnection: %s", err)
					break
				}
				ln.peerConnectionChan <- pc
			}
		}
		close(ln.peerConnectionChan)
	}()

	return ln, nil
}

func handler(conn net.Conn) error {
	defer conn.Close()

	handlerChan <- 1
	defer func() {
		handlerChan <- -1
	}()

	or, err := pt.DialOr(&ptInfo, conn.RemoteAddr().String(), "webrtc")
	if err != nil {
		return err
	}
	defer or.Close()

	copyLoop(conn, or)

	return nil
}

func acceptLoop(ln net.Listener) error {
	defer ln.Close()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if e, ok := err.(net.Error); ok && e.Temporary() {
				continue
			}
			return err
		}
		go handler(conn)
	}
}

func readSignalingMessages(f *os.File) {
	s := bufio.NewScanner(f)
	for s.Scan() {
		msg := s.Text()
		sdp := webrtc.DeserializeSessionDescription(msg)
		if sdp == nil {
			log.Printf("ignoring invalid signal message %q", msg)
			continue
		}
		signalChan <- sdp
	}
	close(signalChan)
	if err := s.Err(); err != nil {
		log.Printf("signal FIFO: %s", err)
	}
}

func main() {
	var err error

	logFile, err = os.OpenFile("webrtc-server.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		log.Fatal(err)
	}
	defer logFile.Close()
	log.SetOutput(logFile)

	log.Println("starting")

	webrtc.SetLoggingVerbosity(1)

	ptInfo, err = pt.ServerSetup(nil)
	if err != nil {
		os.Exit(1)
	}

	webRTCConfig := webrtc.NewConfiguration(webrtc.OptionIceServer("stun:stun.l.google.com:19302"))

	listeners := make([]net.Listener, 0)
	for _, bindaddr := range ptInfo.Bindaddrs {
		switch bindaddr.MethodName {
		case "webrtc":
			// Ignore bindaddr.Addr.
			ln, err := listenWebRTC(webRTCConfig)
			if err != nil {
				pt.SmethodError(bindaddr.MethodName, err.Error())
				break
			}
			go acceptLoop(ln)
			pt.Smethod(bindaddr.MethodName, ln.Addr())
			listeners = append(listeners, ln)
		default:
			pt.SmethodError(bindaddr.MethodName, "no such method")
		}
	}
	pt.SmethodsDone()

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

	if sig == syscall.SIGTERM {
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
