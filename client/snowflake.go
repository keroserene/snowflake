// Client transport plugin for the Snowflake pluggable transport.
package main

import (
	"flag"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	pt "git.torproject.org/pluggable-transports/goptlib.git"
	sf "git.torproject.org/pluggable-transports/snowflake.git/client/lib"
	"git.torproject.org/pluggable-transports/snowflake.git/common/nat"
	"git.torproject.org/pluggable-transports/snowflake.git/common/safelog"
	"github.com/pion/webrtc/v2"
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
		if err != nil {
			log.Printf("WebRTC: %v  Retrying in %v...",
				err, sf.ReconnectTimeout)
		}
		select {
		case <-time.After(sf.ReconnectTimeout):
			continue
		case <-snowflakes.Melted():
			log.Println("ConnectLoop: stopped.")
			return
		}
	}
}

// Accept local SOCKS connections and pass them to the handler.
func socksAcceptLoop(ln *pt.SocksListener, snowflakes sf.SnowflakeCollector) {
	defer ln.Close()
	for {
		conn, err := ln.AcceptSocks()
		if err != nil {
			if err, ok := err.(net.Error); ok && err.Temporary() {
				continue
			}
			log.Printf("SOCKS accept error: %s", err)
			break
		}
		log.Printf("SOCKS accepted: %v", conn.Req)
		go func() {
			defer conn.Close()

			err := conn.Grant(&net.TCPAddr{IP: net.IPv4zero, Port: 0})
			if err != nil {
				log.Printf("conn.Grant error: %s", err)
				return
			}

			err = sf.Handler(conn, snowflakes)
			if err != nil {
				log.Printf("handler error: %s", err)
				return
			}
		}()
	}
}

// s is a comma-separated list of ICE server URLs.
func parseIceServers(s string) []webrtc.ICEServer {
	var servers []webrtc.ICEServer
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return nil
	}
	urls := strings.Split(s, ",")
	for _, url := range urls {
		url = strings.TrimSpace(url)
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
	logToStateDir := flag.Bool("log-to-state-dir", false, "resolve the log file relative to tor's pt state dir")
	keepLocalAddresses := flag.Bool("keep-local-addresses", false, "keep local LAN address ICE candidates")
	unsafeLogging := flag.Bool("unsafe-logging", false, "prevent logs from being scrubbed")
	max := flag.Int("max", DefaultSnowflakeCapacity,
		"capacity for number of multiplexed WebRTC peers")

	// Deprecated
	oldLogToStateDir := flag.Bool("logToStateDir", false, "use -log-to-state-dir instead")
	oldKeepLocalAddresses := flag.Bool("keepLocalAddresses", false, "use -keep-local-addresses instead")

	flag.Parse()

	log.SetFlags(log.LstdFlags | log.LUTC)

	// Don't write to stderr; versions of tor earlier than about 0.3.5.6 do
	// not read from the pipe, and eventually we will deadlock because the
	// buffer is full.
	// https://bugs.torproject.org/26360
	// https://bugs.torproject.org/25600#comment:14
	var logOutput = ioutil.Discard
	if *logFilename != "" {
		if *logToStateDir || *oldLogToStateDir {
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
	if *unsafeLogging {
		log.SetOutput(logOutput)
	} else {
		// We want to send the log output through our scrubber first
		log.SetOutput(&safelog.LogScrubber{Output: logOutput})
	}

	log.Println("\n\n\n --- Starting Snowflake Client ---")

	iceServers := parseIceServers(*iceServersCommas)
	// chooses a random subset of servers from inputs
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(iceServers), func(i, j int) {
		iceServers[i], iceServers[j] = iceServers[j], iceServers[i]
	})
	if len(iceServers) > 2 {
		iceServers = iceServers[:(len(iceServers)+1)/2]
	}
	log.Printf("Using ICE servers:")
	for _, server := range iceServers {
		log.Printf("url: %v", strings.Join(server.URLs, " "))
	}

	// Prepare to collect remote WebRTC peers.
	snowflakes := sf.NewPeers(*max)

	// Use potentially domain-fronting broker to rendezvous.
	broker, err := sf.NewBrokerChannel(
		*brokerURL, *frontDomain, sf.CreateBrokerTransport(),
		*keepLocalAddresses || *oldKeepLocalAddresses)
	if err != nil {
		log.Fatalf("parsing broker URL: %v", err)
	}
	go updateNATType(iceServers, broker)

	snowflakes.Tongue = sf.NewWebRTCDialer(broker, iceServers)

	// Use a real logger to periodically output how much traffic is happening.
	snowflakes.BytesLogger = sf.NewBytesSyncLogger()

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
			log.Printf("Started SOCKS listener at %v.", ln.Addr())
			go socksAcceptLoop(ln, snowflakes)
			pt.Cmethod(methodName, ln.Version(), ln.Addr())
			listeners = append(listeners, ln)
		default:
			pt.CmethodError(methodName, "no such method")
		}
	}
	pt.CmethodsDone()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM)

	if os.Getenv("TOR_PT_EXIT_ON_STDIN_CLOSE") == "1" {
		// This environment variable means we should treat EOF on stdin
		// just like SIGTERM: https://bugs.torproject.org/15435.
		go func() {
			if _, err := io.Copy(ioutil.Discard, os.Stdin); err != nil {
				log.Printf("calling io.Copy(ioutil.Discard, os.Stdin) returned error: %v", err)
			}
			log.Printf("synthesizing SIGTERM because of stdin close")
			sigChan <- syscall.SIGTERM
		}()
	}

	// Wait for a signal.
	<-sigChan

	// Signal received, shut down.
	for _, ln := range listeners {
		ln.Close()
	}
	snowflakes.End()
	log.Println("snowflake is done.")
}

// loop through all provided STUN servers until we exhaust the list or find
// one that is compatable with RFC 5780
func updateNATType(servers []webrtc.ICEServer, broker *sf.BrokerChannel) {

	var restrictedNAT bool
	var err error
	for _, server := range servers {
		addr := strings.TrimPrefix(server.URLs[0], "stun:")
		restrictedNAT, err = nat.CheckIfRestrictedNAT(addr)
		if err == nil {
			if restrictedNAT {
				broker.SetNATType(nat.NATRestricted)
			} else {
				broker.SetNATType(nat.NATUnrestricted)
			}
			break
		}
	}
	if err != nil {
		broker.SetNATType(nat.NATUnknown)
	}
}
