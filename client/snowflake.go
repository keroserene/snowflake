// Client transport plugin for the Snowflake pluggable transport.
package main

import (
	"errors"
	"flag"
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

const (
	ReconnectTimeout         = 10
	DefaultSnowflakeCapacity = 1
)

// When a connection handler starts, +1 is written to this channel; when it
// ends, -1 is written.
var handlerChan = make(chan int)

// Maintain |SnowflakeCapacity| number of available WebRTC connections, to
// transfer to the Tor SOCKS handler when needed.
func ConnectLoop(snowflakes SnowflakeCollector) {
	for {
		// Check if ending is necessary.
		_, err := snowflakes.Collect()
		if nil != err {
			log.Println("WebRTC:", err,
				" Retrying in", ReconnectTimeout, "seconds...")
		}
		select {
		case <-time.After(time.Second * ReconnectTimeout):
			continue
		case <-snowflakes.Melted():
			log.Println("ConnectLoop: stopped.")
			return
		}
	}
}

// Accept local SOCKS connections and pass them to the handler.
func socksAcceptLoop(ln *pt.SocksListener, snowflakes SnowflakeCollector) error {
	defer ln.Close()
	log.Println("Started SOCKS listener.")
	for {
		log.Println("SOCKS listening...")
		conn, err := ln.AcceptSocks()
		log.Println("SOCKS accepted: ", conn.Req)
		if err != nil {
			if e, ok := err.(net.Error); ok && e.Temporary() {
				continue
			}
			return err
		}
		err = handler(conn, snowflakes)
		if err != nil {
			log.Printf("handler error: %s", err)
		}
	}
}

// Given an accepted SOCKS connection, establish a WebRTC connection to the
// remote peer and exchange traffic.
func handler(socks SocksConnector, snowflakes SnowflakeCollector) error {
	handlerChan <- 1
	defer func() {
		handlerChan <- -1
	}()
	// Obtain an available WebRTC remote. May block.
	log.Println("handler: awaiting Snowflake...")
	snowflake := snowflakes.Pop()
	if nil == snowflake {
		socks.Reject()
		return errors.New("handler: Received invalid Snowflake")
	}
	defer socks.Close()
	log.Println("---- Handler: snowflake assigned ----")
	err := socks.Grant(&net.TCPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return err
	}

	go func() {
		// When WebRTC resets, close the SOCKS connection, which ends
		// the copyLoop below and induces new handler.
		snowflake.WaitForReset()
		socks.Close()
	}()

	// Begin exchanging data.
	copyLoop(socks, snowflake)
	log.Println("---- Handler: closed ---")
	return nil
}

// Exchanges bytes between two ReadWriters.
// (In this case, between a SOCKS and WebRTC connection.)
func copyLoop(a, b io.ReadWriter) {
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
	log.Println("copy loop ended")
}

func main() {
	webrtc.SetLoggingVerbosity(1)
	logFile, err := os.OpenFile("snowflake.log",
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		log.Fatal(err)
	}
	defer logFile.Close()
	log.SetOutput(logFile)

	var iceServers IceServerList
	log.Println("\n\n\n --- Starting Snowflake Client ---")

	flag.Var(&iceServers, "ice", "comma-separated list of ICE servers")
	brokerURL := flag.String("url", "", "URL of signaling broker")
	frontDomain := flag.String("front", "", "front domain")
	max := flag.Int("max", DefaultSnowflakeCapacity,
		"capacity for number of multiplexed WebRTC peers")
	flag.Parse()

	// Prepare to collect remote WebRTC peers.
	snowflakes := NewPeers(*max)
	if "" != *brokerURL {
		// Use potentially domain-fronting broker to rendezvous.
		broker := NewBrokerChannel(*brokerURL, *frontDomain, CreateBrokerTransport())
		snowflakes.Tongue = NewWebRTCDialer(broker, iceServers)
	} else {
		// Otherwise, use manual copy and pasting of SDP messages.
		snowflakes.Tongue = NewCopyPasteDialer(iceServers)
	}
	// Use a real logger to periodically output how much traffic is happening.
	snowflakes.BytesLogger = &BytesSyncLogger{
		inboundChan: make(chan int, 5), outboundChan: make(chan int, 5),
		inbound: 0, outbound: 0, inEvents: 0, outEvents: 0,
	}
	go snowflakes.BytesLogger.Log()

	go ConnectLoop(snowflakes)

	// Begin goptlib client process.
	ptInfo, err := pt.ClientSetup(nil)
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
			// TODO: Be able to recover when SOCKS dies.
			ln, err := pt.ListenSocks("tcp", "127.0.0.1:0")
			if err != nil {
				pt.CmethodError(methodName, err.Error())
				break
			}
			go socksAcceptLoop(ln, snowflakes)
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
	snowflakes.End()
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
