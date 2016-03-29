// Client transport plugin for the Snowflake pluggable transport.
package main

import (
	"bufio"
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

var ptInfo pt.ClientInfo

const (
	ReconnectTimeout  = 5
	SnowflakeCapacity = 3
)

var brokerURL string
var frontDomain string
var iceServers IceServerList
var snowflakeChan = make(chan *webRTCConn, 1)
var broker *BrokerChannel

// When a connection handler starts, +1 is written to this channel; when it
// ends, -1 is written.
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
	log.Println("copy loop ended")
}

// Interface that matches both webrtc.DataChannel and for testing.
type SnowflakeChannel interface {
	Send([]byte)
	Close() error
}

// Maintain |SnowflakeCapacity| number of available WebRTC connections, to
// transfer to the Tor SOCKS handler when needed.
func SnowflakeConnectLoop() {
	broker = NewBrokerChannel(brokerURL, frontDomain)
	for {
		numRemotes := len(webrtcRemotes)
		if numRemotes >= SnowflakeCapacity {
			log.Println("At Capacity: ", numRemotes, "snowflake. Re-checking in 10s")
			<-time.After(time.Second * 10)
			continue
		}
		s, err := dialWebRTC()
		if nil == s || nil != err {
			log.Println("WebRTC Error: ", err, " retrying...")
			<-time.After(time.Second * ReconnectTimeout)
			continue
		}
		snowflakeChan <- s
	}
}

// Initialize a WebRTC Connection.
func dialWebRTC() (*webRTCConn, error) {
	// TODO: [#3] Fetch ICE server information from Broker.
	// TODO: [#18] Consider TURN servers here too.
	config := webrtc.NewConfiguration(iceServers...)
	if nil == broker {
		return nil, errors.New("Failed to prepare BrokerChannel")
	}
	connection := NewWebRTCConnection(config, broker)
	err := connection.Connect()
	return connection, err
}

func endWebRTC() {
	log.Printf("WebRTC: interruped")
	for _, r := range webrtcRemotes {
		r.Close()
	}
}

// Establish a WebRTC channel for SOCKS connections.
func handler(conn *pt.SocksConn) error {
	handlerChan <- 1
	defer func() {
		handlerChan <- -1
	}()
	// Wait for an available WebRTC remote...
	remote, ok := <-snowflakeChan
	if remote == nil || !ok {
		conn.Reject()
		return errors.New("handler: Received invalid Snowflake")
	}
	defer remote.Close()
	defer conn.Close()
	log.Println("handler: Snowflake assigned.")

	err := conn.Grant(&net.TCPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return err
	}

	go copyLoop(conn, remote)
	// When WebRTC resets, close the SOCKS connection, which induces new handler.
	<-remote.reset
	log.Println("---- Closed ---")
	return nil
}

func acceptLoop(ln *pt.SocksListener) error {
	defer ln.Close()
	for {
		log.Println("SOCKS listening...", ln)
		conn, err := ln.AcceptSocks()
		log.Println("accepting", conn, err)
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
		webrtcRemotes[0].answerChannel <- sdp
	}
	log.Printf("close answerChannel")
	close(webrtcRemotes[0].answerChannel)
	if err := s.Err(); err != nil {
		log.Printf("signal FIFO: %s", err)
	}
}

func main() {
	webrtc.SetLoggingVerbosity(1)
	logFile, err := os.OpenFile("snowflake.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		log.Fatal(err)
	}
	defer logFile.Close()
	log.SetOutput(logFile)
	log.Println("\nStarting Snowflake Client...")

	flag.StringVar(&brokerURL, "url", "", "URL of signaling broker")
	flag.StringVar(&frontDomain, "front", "", "front domain")
	flag.Var(&iceServers, "ice", "comma-separated list of ICE servers")
	flag.Parse()

	// TODO: Maybe just get rid of copy-paste option entirely.
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

	webrtcRemotes = make(map[int]*webRTCConn)
	go SnowflakeConnectLoop()

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
