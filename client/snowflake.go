// Client transport plugin for the Snowflake pluggable transport.
// In the Client context, "Snowflake" refers to a remote browser proxy.
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
)

var ptInfo pt.ClientInfo

const (
	ReconnectTimeout  = 10
	SnowflakeCapacity = 1
)

var brokerURL string
var frontDomain string
var iceServers IceServerList

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

// Collect and track available snowflakes. (Implements SnowflakeCollector)
// Right now, it is only possible to use one active remote in a circuit.
// This can be updated once multiplexed transport on a single circuit is available.
// Keeping multiple WebRTC connections available allows for quicker recovery when
// the current snowflake disconnects.
type SnowflakeJar struct {
	Tongue
	BytesLogger

	snowflakeChan chan *webRTCConn
	current       *webRTCConn
	capacity      int
	maxedChan     chan struct{}
}

func NewSnowflakeJar(max int) *SnowflakeJar {
	p := &SnowflakeJar{capacity: max}
	p.snowflakeChan = make(chan *webRTCConn, max)
	p.maxedChan = make(chan struct{}, 1)
	return p
}

// Establish connection to some remote WebRTC peer, and keep it available for
// later.
func (jar *SnowflakeJar) Collect() (*webRTCConn, error) {
	if jar.Count() >= jar.capacity {
		s := fmt.Sprintf("At capacity [%d/%d]", jar.Count(), jar.capacity)
		jar.maxedChan <- struct{}{}
		return nil, errors.New(s)
	}
	snowflake, err := jar.Catch()
	if err != nil {
		return nil, err
	}
	jar.snowflakeChan <- snowflake
	return snowflake, nil
}

// Prepare and present an available remote WebRTC peer for active use.
func (jar *SnowflakeJar) Release() *webRTCConn {
	snowflake, ok := <-jar.snowflakeChan
	if !ok {
		return nil
	}
	jar.current = snowflake
	snowflake.BytesLogger = jar.BytesLogger
	return snowflake
}

// TODO: Needs fixing.
func (p *SnowflakeJar) Count() int {
	count := 0
	if p.current != nil {
		count = 1
	}
	return count + len(p.snowflakeChan)
}

// Close all remote peers.
func (p *SnowflakeJar) End() {
	log.Printf("WebRTC: interruped")
	if nil != p.current {
		p.current.Close()
	}
	for r := range p.snowflakeChan {
		r.Close()
	}
}

// Maintain |SnowflakeCapacity| number of available WebRTC connections, to
// transfer to the Tor SOCKS handler when needed.
func ConnectLoop(snowflakes SnowflakeCollector) {
	for {
		s, err := snowflakes.Collect()
		if nil == s || nil != err {
			log.Println("WebRTC Error:", err,
				" Retrying in", ReconnectTimeout, "seconds...")
			// Failed collections get a timeout.
			<-time.After(time.Second * ReconnectTimeout)
			continue
		}
		// Successful collection gets rate limited to once per second.
		<-time.After(time.Second)
	}
}

// Accept local SOCKS connections and pass them to the handler.
func acceptLoop(ln *pt.SocksListener, snowflakes SnowflakeCollector) error {
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
	// Wait for an available WebRTC remote...
	snowflake := snowflakes.Release()
	if nil == snowflake {
		socks.Reject()
		return errors.New("handler: Received invalid Snowflake")
	}
	defer socks.Close()
	log.Println("handler: Snowflake assigned.")
	err := socks.Grant(&net.TCPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return err
	}

	// Begin exchanging data.
	go copyLoop(socks, snowflake)

	// When WebRTC resets, close the SOCKS connection, which induces new handler.
	// TODO: Double check this / fix it.
	<-snowflake.reset
	log.Println("---- Closed ---")
	return nil
}

// TODO: Fix since multiplexing changes access to remotes.
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
		// webrtcRemotes[0].answerChannel <- sdp
	}
	log.Printf("close answerChannel")
	// close(webrtcRemotes[0].answerChannel)
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

	// Prepare WebRTC SnowflakeCollector and the Broker, then accumulate connections.
	// TODO: Expose remote peer capacity as a flag?
	snowflakes := NewSnowflakeJar(SnowflakeCapacity)

	broker := NewBrokerChannel(brokerURL, frontDomain, CreateBrokerTransport())
	snowflakes.Tongue = WebRTCDialer{broker}

	// Use a real logger for traffic.
	snowflakes.BytesLogger = &BytesSyncLogger{
		inboundChan: make(chan int, 5), outboundChan: make(chan int, 5),
		inbound: 0, outbound: 0, inEvents: 0, outEvents: 0,
	}

	go ConnectLoop(snowflakes)
	go snowflakes.BytesLogger.Log()

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
			// TODO: Be able to recover when SOCKS dies.
			ln, err := pt.ListenSocks("tcp", "127.0.0.1:0")
			if err != nil {
				pt.CmethodError(methodName, err.Error())
				break
			}
			go acceptLoop(ln, snowflakes)
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
