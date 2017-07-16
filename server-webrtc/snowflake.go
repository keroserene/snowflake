package main

import (
	"bufio"
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

var ptMethodName = "snowflake"
var ptInfo pt.ServerInfo
var logFile *os.File

// When a datachannel handler starts, +1 is written to this channel;
// when it ends, -1 is written.
var handlerChan = make(chan int)

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
	dc *webrtc.DataChannel
	pc *webrtc.PeerConnection
	pr *io.PipeReader
}

func (c *webRTCConn) Read(b []byte) (int, error) {
	return c.pr.Read(b)
}

func (c *webRTCConn) Write(b []byte) (int, error) {
	// log.Printf("webrtc Write %d %+q", len(b), string(b))
	log.Printf("Write %d bytes --> WebRTC", len(b))
	c.dc.Send(b)
	return len(b), nil
}

func (c *webRTCConn) Close() error {
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

func datachannelHandler(conn *webRTCConn) {
	defer conn.Close()

	handlerChan <- 1
	defer func() {
		handlerChan <- -1
	}()

	or, err := pt.DialOr(&ptInfo, "", ptMethodName) // TODO: Extended OR
	if err != nil {
		log.Printf("Failed to connect to ORPort: " + err.Error())
		return
	}
	defer or.Close()

	copyLoop(conn, or)
}

// Create a PeerConnection from an SDP offer. Blocks until the gathering of ICE
// candidates is complete and the answer is available in LocalDescription.
// Installs an OnDataChannel callback that creates a webRTCConn and passes it to
// datachannelHandler.
func makePeerConnectionFromOffer(sdp *webrtc.SessionDescription, config *webrtc.Configuration) (*webrtc.PeerConnection, error) {
	errChan := make(chan error)
	answerChan := make(chan struct{})

	pc, err := webrtc.NewPeerConnection(config)
	if err != nil {
		return nil, fmt.Errorf("accept: NewPeerConnection: %s", err)
	}
	pc.OnNegotiationNeeded = func() {
		panic("OnNegotiationNeeded")
	}
	pc.OnIceComplete = func() {
		answerChan <- struct{}{}
	}
	pc.OnDataChannel = func(dc *webrtc.DataChannel) {
		log.Println("OnDataChannel")

		pr, pw := io.Pipe()

		dc.OnOpen = func() {
			log.Println("OnOpen channel")
		}
		dc.OnClose = func() {
			log.Println("OnClose channel")
			pw.Close()
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

		conn := &webRTCConn{pc: pc, dc: dc, pr: pr}
		go datachannelHandler(conn)
	}

	err = pc.SetRemoteDescription(sdp)
	if err != nil {
		pc.Close()
		return nil, fmt.Errorf("accept: SetRemoteDescription: %s", err)
	}
	log.Println("sdp offer successfully received.")

	go func() {
		log.Println("Generating answer...")
		answer, err := pc.CreateAnswer() // blocking
		if err != nil {
			errChan <- err
			return
		}
		err = pc.SetLocalDescription(answer)
		if err != nil {
			errChan <- err
			return
		}
	}()

	// Wait until answer is ready.
	select {
	case err = <-errChan:
		pc.Close()
		return nil, err
	case _, ok := <-answerChan:
		if !ok {
			pc.Close()
			return nil, fmt.Errorf("Failed gathering ICE candidates.")
		}
	}

	return pc, nil
}

// Create a signaling named pipe and feed offers from it into
// makePeerConnectionFromOffer.
func receiveSignalsFIFO(filename string, config *webrtc.Configuration) error {
	err := syscall.Mkfifo(filename, 0600)
	if err != nil {
		if err.(syscall.Errno) != syscall.EEXIST {
			return err
		}
	}
	signalFile, err := os.OpenFile(filename, os.O_RDONLY, 0600)
	if err != nil {
		return err
	}
	defer signalFile.Close()

	s := bufio.NewScanner(signalFile)
	for s.Scan() {
		msg := s.Text()
		sdp := webrtc.DeserializeSessionDescription(msg)
		if sdp == nil {
			log.Printf("ignoring invalid signal message %+q", msg)
			continue
		}

		pc, err := makePeerConnectionFromOffer(sdp, config)
		if err != nil {
			log.Printf("makePeerConnectionFromOffer: %s", err)
			continue
		}
		// Write offer to log for manual signaling.
		log.Printf("----------------")
		fmt.Fprintln(logFile, pc.LocalDescription().Serialize())
		log.Printf("----------------")
	}
	return s.Err()
}

func main() {
	var err error
	var httpAddr string
	var logFilename string

	flag.StringVar(&httpAddr, "http", "", "listen for HTTP signaling")
	flag.StringVar(&logFilename, "log", "", "log file to write to")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.LUTC)
	if logFilename != "" {
		f, err := os.OpenFile(logFilename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			log.Fatalf("can't open log file: %s", err)
		}
		defer logFile.Close()
		log.SetOutput(f)
	}

	log.Println("starting")
	webrtc.SetLoggingVerbosity(1)

	ptInfo, err = pt.ServerSetup(nil)
	if err != nil {
		log.Fatal(err)
	}

	webRTCConfig := webrtc.NewConfiguration(webrtc.OptionIceServer("stun:stun.l.google.com:19302"))

	// Start FIFO-based signaling receiver.
	go func() {
		err := receiveSignalsFIFO("signal", webRTCConfig)
		if err != nil {
			log.Printf("receiveSignalsFIFO: %s", err)
		}
	}()

	// Start HTTP-based signaling receiver.
	if httpAddr != "" {
		go func() {
			err := receiveSignalsHTTP(httpAddr, webRTCConfig)
			if err != nil {
				log.Printf("receiveSignalsHTTP: %s", err)
			}
		}()
	}

	for _, bindaddr := range ptInfo.Bindaddrs {
		switch bindaddr.MethodName {
		case ptMethodName:
			bindaddr.Addr.Port = 12345 // lies!!!
			pt.Smethod(bindaddr.MethodName, bindaddr.Addr)
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
