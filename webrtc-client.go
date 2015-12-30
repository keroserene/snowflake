package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/keroserene/go-webrtc"
	"github.com/keroserene/go-webrtc/data"

	"git.torproject.org/pluggable-transports/goptlib.git"
)

var ptInfo pt.ClientInfo
var logFile *os.File

// When a connection handler starts, +1 is written to this channel; when it
// ends, -1 is written.
var handlerChan = make(chan int)

func handler(conn *pt.SocksConn) error {
	handlerChan <- 1
	defer func() {
		handlerChan <- -1
	}()
	defer conn.Close()

	config := webrtc.NewConfiguration(webrtc.OptionIceServer("stun:stun.l.google.com:19302"))
	pc, err := webrtc.NewPeerConnection(config)
	if err != nil {
		log.Printf("NewPeerConnection: %s", err)
		return err
	}

	// For now, the Go client is always the offerer.
	// TODO: Copy paste signaling

	pc.OnNegotiationNeeded = func() {
		// log.Println("OnNegotiationNeeded")
		go func() {
			offer, err := pc.CreateOffer()
			if err != nil {
				log.Printf("CreateOffer: %s", err)
				return
			}
			fmt.Fprintln(logFile, offer.Serialize())
			pc.SetLocalDescription(offer)
		}()
	}
	pc.OnIceCandidate = func(candidate webrtc.IceCandidate) {
		// log.Printf("OnIceCandidate %q", candidate.Candidate)
		fmt.Fprintln(logFile, candidate.Serialize())
	}
	pc.OnDataChannel = func(channel *data.Channel) {
		log.Println("OnDataChannel")
		panic("OnDataChannel")
	}

	dc, err := pc.CreateDataChannel("test", data.Init{})
	if err != nil {
		log.Printf("CreateDataChannel: %s", err)
		return err
	}
	dc.OnOpen = func() {
		log.Println("OnOpen channel")
	}
	dc.OnClose = func() {
		log.Println("OnClose channel")
	}
	dc.OnMessage = func(msg []byte) {
		log.Println("OnMessage channel %d %q", len(msg), msg)
	}

	// defer close channel?

	err = conn.Grant(&net.TCPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return err
	}

	<-make(chan int)

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
		go handler(conn)
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
