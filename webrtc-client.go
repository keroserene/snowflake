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

var ptInfo pt.ClientInfo
var logFile *os.File

var notImplemented = fmt.Errorf("not implemented")

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

func dialWebRTC(config *webrtc.Configuration) (*webRTCConn, error) {
	blobChan := make(chan string)
	errChan := make(chan error)
	openChan := make(chan bool)

	pc, err := webrtc.NewPeerConnection(config)
	if err != nil {
		log.Printf("NewPeerConnection: %s", err)
		return nil, err
	}

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
	pc.OnIceComplete = func() {
		log.Printf("OnIceComplete")
		blobChan <- pc.LocalDescription().Serialize()
	}
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
		openChan <- true
	}
	dc.OnClose = func() {
		log.Println("OnClose channel")
		pw.Close()
		close(openChan)
	}
	dc.OnMessage = func(msg []byte) {
		log.Printf("OnMessage channel %d %q", len(msg), msg)
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
	case offer := <-blobChan:
		log.Printf("----------------")
		fmt.Fprintln(logFile, offer)
		log.Printf("----------------")
	}

	log.Printf("waiting for answer")
	answer, ok := <-signalChan

	if !ok {
		pc.Close()
		return nil, fmt.Errorf("no answer received")
	}
	log.Printf("got answer %s", answer.Serialize())
	err = pc.SetRemoteDescription(answer)
	if err != nil {
		pc.Close()
		return nil, err
	}

	// Wait until data channel is open; otherwise for example sends may get
	// lost.
	_, ok = <-openChan
	if !ok {
		pc.Close()
		return nil, fmt.Errorf("failed to open data channel")
	}

	return &webRTCConn{pc: pc, dc: dc, recvPipe: pr}, nil
}

func handler(conn *pt.SocksConn) error {
	handlerChan <- 1
	defer func() {
		handlerChan <- -1
	}()
	defer conn.Close()

	config := webrtc.NewConfiguration(webrtc.OptionIceServer("stun:stun.l.google.com:19302"))
	remote, err := dialWebRTC(config)
	if err != nil {
		conn.Reject()
		return err
	}
	defer remote.Close()

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
		log.Printf("readSignalingMessages loop %q", msg)
		sdp := webrtc.DeserializeSessionDescription(msg)
		if sdp == nil {
			log.Printf("ignoring invalid signal message %q", msg)
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

	logFile, err = os.OpenFile("webrtc-client.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		log.Fatal(err)
	}
	defer logFile.Close()
	log.SetOutput(logFile)

	log.Println("starting")

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

	webrtc.SetLoggingVerbosity(1)

	ptInfo, err = pt.ClientSetup(nil)
	if err != nil {
		os.Exit(1)
	}

	if ptInfo.ProxyURL != nil {
		pt.ProxyError("proxy is not supported")
		os.Exit(1)
	}

	listeners := make([]net.Listener, 0)
	for _, methodName := range ptInfo.MethodNames {
		switch methodName {
		case "webrtc":
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
