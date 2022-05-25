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
	"strconv"
	"strings"
	"sync"
	"syscall"

	pt "git.torproject.org/pluggable-transports/goptlib.git"
	sf "git.torproject.org/pluggable-transports/snowflake.git/v2/client/lib"
	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/event"
	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/safelog"
)

const (
	DefaultSnowflakeCapacity = 1
)

type ptEventLogger struct {
}

func NewPTEventLogger() event.SnowflakeEventReceiver {
	return &ptEventLogger{}
}

func (p ptEventLogger) OnNewSnowflakeEvent(e event.SnowflakeEvent) {
	pt.Log(pt.LogSeverityNotice, e.String())
}

// Exchanges bytes between two ReadWriters.
// (In this case, between a SOCKS connection and a snowflake transport conn)
func copyLoop(socks, sfconn io.ReadWriter) {
	done := make(chan struct{}, 2)
	go func() {
		if _, err := io.Copy(socks, sfconn); err != nil {
			log.Printf("copying Snowflake to SOCKS resulted in error: %v", err)
		}
		done <- struct{}{}
	}()
	go func() {
		if _, err := io.Copy(sfconn, socks); err != nil {
			log.Printf("copying SOCKS to Snowflake resulted in error: %v", err)
		}
		done <- struct{}{}
	}()
	<-done
	log.Println("copy loop ended")
}

// Accept local SOCKS connections and connect to a Snowflake connection
func socksAcceptLoop(ln *pt.SocksListener, config sf.ClientConfig, shutdown chan struct{}, wg *sync.WaitGroup) {
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
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer conn.Close()

			// Check to see if our command line options are overriden by SOCKS options
			if arg, ok := conn.Req.Args.Get("ampcache"); ok {
				config.AmpCacheURL = arg
			}
			if arg, ok := conn.Req.Args.Get("front"); ok {
				config.FrontDomain = arg
			}
			if arg, ok := conn.Req.Args.Get("ice"); ok {
				config.ICEAddresses = strings.Split(strings.TrimSpace(arg), ",")
			}
			if arg, ok := conn.Req.Args.Get("max"); ok {
				max, err := strconv.Atoi(arg)
				if err != nil {
					conn.Reject()
					log.Println("Invalid SOCKS arg: max=", arg)
					return
				}
				config.Max = max
			}
			if arg, ok := conn.Req.Args.Get("url"); ok {
				config.BrokerURL = arg
			}
			if arg, ok := conn.Req.Args.Get("utls-nosni"); ok {
				switch strings.ToLower(arg) {
				case "true":
					fallthrough
				case "yes":
					config.UTLSRemoveSNI = true
				}
			}
			if arg, ok := conn.Req.Args.Get("utls-imitate"); ok {
				config.UTLSClientID = arg
			}
			if arg, ok := conn.Req.Args.Get("fingerprint"); ok {
				config.BridgeFingerprint = arg
			}
			transport, err := sf.NewSnowflakeClient(config)
			if err != nil {
				conn.Reject()
				log.Println("Failed to start snowflake transport: ", err)
				return
			}
			transport.AddSnowflakeEventListener(NewPTEventLogger())
			err = conn.Grant(&net.TCPAddr{IP: net.IPv4zero, Port: 0})
			if err != nil {
				log.Printf("conn.Grant error: %s", err)
				return
			}

			handler := make(chan struct{})
			go func() {
				defer close(handler)
				sconn, err := transport.Dial()
				if err != nil {
					log.Printf("dial error: %s", err)
					return
				}
				defer sconn.Close()
				// copy between the created Snowflake conn and the SOCKS conn
				copyLoop(conn, sconn)
			}()
			select {
			case <-shutdown:
				log.Println("Received shutdown signal")
			case <-handler:
				log.Println("Handler ended")
			}
			return
		}()
	}
}

func main() {
	iceServersCommas := flag.String("ice", "", "comma-separated list of ICE servers")
	brokerURL := flag.String("url", "", "URL of signaling broker")
	frontDomain := flag.String("front", "", "front domain")
	ampCacheURL := flag.String("ampcache", "", "URL of AMP cache to use as a proxy for signaling")
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

	iceAddresses := strings.Split(strings.TrimSpace(*iceServersCommas), ",")

	config := sf.ClientConfig{
		BrokerURL:          *brokerURL,
		AmpCacheURL:        *ampCacheURL,
		FrontDomain:        *frontDomain,
		ICEAddresses:       iceAddresses,
		KeepLocalAddresses: *keepLocalAddresses || *oldKeepLocalAddresses,
		Max:                *max,
	}

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
	shutdown := make(chan struct{})
	var wg sync.WaitGroup
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
			go socksAcceptLoop(ln, config, shutdown, &wg)
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
	log.Println("stopping snowflake")

	// Signal received, shut down.
	for _, ln := range listeners {
		ln.Close()
	}
	close(shutdown)
	wg.Wait()
	log.Println("snowflake is done.")
}
