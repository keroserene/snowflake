// Client transport plugin for the Snowflake pluggable transport.
package main

import (
	"flag"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"git.torproject.org/pluggable-transports/goptlib.git"
	sf "git.torproject.org/pluggable-transports/snowflake.git/client/lib"
	"git.torproject.org/pluggable-transports/snowflake.git/common/safelog"
	"github.com/pion/webrtc"
)

const (
	DefaultSnowflakeCapacity = 1
)

// Maintain |SnowflakeCapacity| number of available WebRTC connections, to
// transfer to the Tor SOCKS handler when needed.
func ConnectLoop(snowflakes sf.SnowflakeCollector) {
	for {
		// Check if ending is necessary.
		_, err := snowflakes.Collect()
		if nil != err {
			log.Println("WebRTC:", err,
				" Retrying in", sf.ReconnectTimeout, "seconds...")
		}
		select {
		case <-time.After(time.Second * sf.ReconnectTimeout):
			continue
		case <-snowflakes.Melted():
			log.Println("ConnectLoop: stopped.")
			return
		}
	}
}

// Accept local SOCKS connections and pass them to the handler.
func socksAcceptLoop(ln *pt.SocksListener, snowflakes sf.SnowflakeCollector) error {
	defer ln.Close()
	log.Println("Started SOCKS listener.")
	for {
		log.Println("SOCKS listening...")
		conn, err := ln.AcceptSocks()
		if err != nil {
			if e, ok := err.(net.Error); ok && e.Temporary() {
				continue
			}
			return err
		}
		log.Println("SOCKS accepted: ", conn.Req)
		err = sf.Handler(conn, snowflakes)
		if err != nil {
			log.Printf("handler error: %s", err)
		}
	}
}

//s is a comma-separated list of ICE server URLs
func parseIceServers(s string) []webrtc.ICEServer {
	var servers []webrtc.ICEServer
	log.Println(s)
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return nil
	}
	urls := strings.Split(s, ",")
	log.Printf("Using ICE Servers:")
	for _, url := range urls {
		log.Printf("url: %s", url)
		servers = append(servers, webrtc.ICEServer{
			URLs: []string{url},
		})
	}
	return servers
}

func main() {
	iceServersCommas := flag.String("ice", "", "comma-separated list of ICE servers")
	brokerURL := flag.String("url", "", "URL of signaling broker")
	frontDomain := flag.String("front", "", "front domain")
	logFilename := flag.String("log", "", "name of log file")
	logToStateDir := flag.Bool("logToStateDir", false, "resolve the log file relative to tor's pt state dir")
	max := flag.Int("max", DefaultSnowflakeCapacity,
		"capacity for number of multiplexed WebRTC peers")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.LUTC)

	// Don't write to stderr; versions of tor earlier than about
	// 0.3.5.6 do not read from the pipe, and eventually we will
	// deadlock because the buffer is full.
	// https://bugs.torproject.org/26360
	// https://bugs.torproject.org/25600#comment:14
	var logOutput io.Writer = ioutil.Discard
	if *logFilename != "" {
		if *logToStateDir {
			stateDir, err := pt.MakeStateDir()
			if err != nil {
				log.Fatal(err)
			}
			*logFilename = filepath.Join(stateDir, *logFilename)
		}
		logFile, err := os.OpenFile(*logFilename,
			os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			log.Fatal(err)
		}
		defer logFile.Close()
		logOutput = logFile
	}
	//We want to send the log output through our scrubber first
	log.SetOutput(&safelog.LogScrubber{Output: logOutput})

	log.Println("\n\n\n --- Starting Snowflake Client ---")

	iceServers := parseIceServers(*iceServersCommas)

	// Prepare to collect remote WebRTC peers.
	snowflakes := sf.NewPeers(*max)

	// Use potentially domain-fronting broker to rendezvous.
	broker := sf.NewBrokerChannel(*brokerURL, *frontDomain, sf.CreateBrokerTransport())
	snowflakes.Tongue = sf.NewWebRTCDialer(broker, iceServers)

	if nil == snowflakes.Tongue {
		log.Fatal("Unable to prepare rendezvous method.")
		return
	}
	// Use a real logger to periodically output how much traffic is happening.
	snowflakes.BytesLogger = &sf.BytesSyncLogger{
		InboundChan:  make(chan int, 5),
		OutboundChan: make(chan int, 5),
		Inbound:      0,
		Outbound:     0,
		InEvents:     0,
		OutEvents:    0,
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
	signal.Notify(sigChan, syscall.SIGTERM)

	if os.Getenv("TOR_PT_EXIT_ON_STDIN_CLOSE") == "1" {
		// This environment variable means we should treat EOF on stdin
		// just like SIGTERM: https://bugs.torproject.org/15435.
		go func() {
			io.Copy(ioutil.Discard, os.Stdin)
			log.Printf("synthesizing SIGTERM because of stdin close")
			sigChan <- syscall.SIGTERM
		}()
	}

	// keep track of handlers and wait for a signal
	sig = nil
	for sig == nil {
		select {
		case n := <-sf.HandlerChan:
			numHandlers += n
		case sig = <-sigChan:
		}
	}

	// signal received, shut down
	for _, ln := range listeners {
		ln.Close()
	}
	snowflakes.End()
	for numHandlers > 0 {
		numHandlers += <-sf.HandlerChan
	}
	log.Println("snowflake is done.")
}
