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

	"git.torproject.org/pluggable-transports/goptlib.git"
	"github.com/keroserene/go-webrtc"
	"github.com/keroserene/go-webrtc/data"
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
	dc *data.Channel
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
	handlerChan <- 1
	defer func() {
		handlerChan <- -1
	}()

	or, err := pt.DialOr(&ptInfo, "", ptMethodName) // TODO: Extended OR
	if err != nil {
		log.Printf("Failed to connect to ORPort: " + err.Error())
		return
	}
	//defer or.Close()

	pr, pw := io.Pipe()
	conn.pr = pr

	dc := conn.dc
	dc.OnOpen = func() {
		log.Println("OnOpen channel")
	}
	dc.OnClose = func() {
		log.Println("OnClose channel")
		pw.Close()
	}
	dc.OnMessage = func(msg []byte) {
		// log.Printf("OnMessage channel %d %+q", len(msg), msg)
		log.Printf("OnMessage <--- %d bytes", len(msg))
		n, err := pw.Write(msg)
		if err != nil {
			pw.CloseWithError(err)
		}
		if n != len(msg) {
			panic("short write")
		}
	}

	go copyLoop(conn, or)
}

func makePeerConnection(config *webrtc.Configuration) (*webrtc.PeerConnection, error) {
	pc, err := webrtc.NewPeerConnection(config)

	if err != nil {
		log.Printf("NewPeerConnection: %s", err)
		return nil, err
	}
	pc.OnNegotiationNeeded = func() {
		panic("OnNegotiationNeeded")
	}
	pc.OnDataChannel = func(dc *data.Channel) {
		log.Println("OnDataChannel")
		datachannelHandler(&webRTCConn{pc: pc, dc: dc})
	}
	pc.OnIceComplete = func() {
		log.Printf("----------------")
		fmt.Fprintln(logFile, pc.LocalDescription().Serialize())
		log.Printf("----------------")
	}
	return pc, nil
}

func readSignalingMessages(signalChan chan *webrtc.SessionDescription, f *os.File) {
	s := bufio.NewScanner(f)
	for s.Scan() {
		msg := s.Text()
		sdp := webrtc.DeserializeSessionDescription(msg)
		if sdp == nil {
			log.Printf("ignoring invalid signal message %+q", msg)
			continue
		}
		signalChan <- sdp
		continue
	}
	if err := s.Err(); err != nil {
		log.Printf("signal FIFO: %s", err)
	}
}

func generateAnswer(pc *webrtc.PeerConnection) {
	fmt.Println("Generating answer...")
	answer, err := pc.CreateAnswer() // blocking
	if err != nil {
		fmt.Println(err)
		return
	}
	pc.SetLocalDescription(answer)
}

func listenWebRTC(config *webrtc.Configuration, signal string) (*os.File, error) {
	err := syscall.Mkfifo(signal, 0600)
	if err != nil {
		if err.(syscall.Errno) != syscall.EEXIST {
			return nil, err
		}
	}
	signalFile, err := os.OpenFile(signal, os.O_RDONLY, 0600)
	if err != nil {
		return nil, err
	}
	//defer signalFile.Close()

	var signalChan = make(chan *webrtc.SessionDescription)

	go func() {
		for {
			select {
			case sdp := <-signalChan:
				pc, err := makePeerConnection(config)
				if err != nil {
					log.Printf("makePeerConnection: %s", err)
					break
				}
				err = pc.SetRemoteDescription(sdp)
				if err != nil {
					fmt.Println("ERROR", err)
					break
				}
				fmt.Println("sdp offer successfully received.")
				go generateAnswer(pc)
			}
		}
	}()

	go readSignalingMessages(signalChan, signalFile)
	log.Printf("waiting for offer")
	return signalFile, nil
}

func main() {
	var err error

	logFile, err = os.OpenFile("snowflake.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
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

	listeners := make([]*os.File, 0)
	for _, bindaddr := range ptInfo.Bindaddrs {
		switch bindaddr.MethodName {
		case ptMethodName:
			ln, err := listenWebRTC(webRTCConfig, "signal") // meh
			if err != nil {
				pt.SmethodError(bindaddr.MethodName, err.Error())
				break
			}
			bindaddr.Addr.Port = 12345 // lies!!!
			pt.Smethod(bindaddr.MethodName, bindaddr.Addr)
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
