// Snowflake-specific websocket server plugin. It reports the transport name as
// "snowflake".
package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"git.torproject.org/pluggable-transports/goptlib.git"
	"git.torproject.org/pluggable-transports/snowflake.git/common/safelog"
	"git.torproject.org/pluggable-transports/websocket.git/websocket"
	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/net/http2"
)

const ptMethodName = "snowflake"
const requestTimeout = 10 * time.Second

const maxMessageSize = 64 * 1024

// How long to wait for ListenAndServe or ListenAndServeTLS to return an error
// before deciding that it's not going to return.
const listenAndServeErrorTimeout = 100 * time.Millisecond

var ptInfo pt.ServerInfo

// When a connection handler starts, +1 is written to this channel; when it
// ends, -1 is written.
var handlerChan = make(chan int)

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: %s [OPTIONS]

WebSocket server pluggable transport for Snowflake. Works only as a managed
proxy. Uses TLS with ACME (Let's Encrypt) by default. Set the certificate
hostnames with the --acme-hostnames option. Use ServerTransportListenAddr in
torrc to choose the listening port. When using TLS, this program will open an
additional HTTP listener on port 80 to work with ACME.

`, os.Args[0])
	flag.PrintDefaults()
}

// An abstraction that makes an underlying WebSocket connection look like an
// io.ReadWriteCloser.
type webSocketConn struct {
	Ws         *websocket.WebSocket
	messageBuf []byte
}

// Implements io.Reader.
func (conn *webSocketConn) Read(b []byte) (n int, err error) {
	for len(conn.messageBuf) == 0 {
		var m websocket.Message
		m, err = conn.Ws.ReadMessage()
		if err != nil {
			return
		}
		if m.Opcode == 8 {
			err = io.EOF
			return
		}
		if m.Opcode != 2 {
			err = fmt.Errorf("got non-binary opcode %d", m.Opcode)
			return
		}
		conn.messageBuf = m.Payload
	}

	n = copy(b, conn.messageBuf)
	conn.messageBuf = conn.messageBuf[n:]

	return
}

// Implements io.Writer.
func (conn *webSocketConn) Write(b []byte) (int, error) {
	err := conn.Ws.WriteMessage(2, b)
	return len(b), err
}

// Implements io.Closer.
func (conn *webSocketConn) Close() error {
	// Ignore any error in trying to write a Close frame.
	_ = conn.Ws.WriteFrame(8, nil)
	return conn.Ws.Conn.Close()
}

// Create a new webSocketConn.
func newWebSocketConn(ws *websocket.WebSocket) webSocketConn {
	var conn webSocketConn
	conn.Ws = ws
	return conn
}

// Copy from WebSocket to socket and vice versa.
func proxy(local *net.TCPConn, conn *webSocketConn) {
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		if _, err := io.Copy(conn, local); err != nil {
			log.Printf("error copying ORPort to WebSocket %v", err)
		}
		if err := local.CloseRead(); err != nil {
			log.Printf("error closing read after copying ORPort to WebSocket %v", err)
		}
		conn.Close()
		wg.Done()
	}()
	go func() {
		if _, err := io.Copy(local, conn); err != nil {
			log.Printf("error copying WebSocket to ORPort")
		}
		if err := local.CloseWrite(); err != nil {
			log.Printf("error closing write after copying WebSocket to ORPort %v", err)
		}
		conn.Close()
		wg.Done()
	}()

	wg.Wait()
}

// Return an address string suitable to pass into pt.DialOr.
func clientAddr(clientIPParam string) string {
	if clientIPParam == "" {
		return ""
	}
	// Check if client addr is a valid IP
	clientIP := net.ParseIP(clientIPParam)
	if clientIP == nil {
		return ""
	}
	// Add a dummy port number. USERADDR requires a port number.
	return (&net.TCPAddr{IP: clientIP, Port: 1, Zone: ""}).String()
}

func webSocketHandler(ws *websocket.WebSocket) {
	// Undo timeouts on HTTP request handling.
	if err := ws.Conn.SetDeadline(time.Time{}); err != nil {
		log.Printf("unable to set deadlines with error: %v", err)
	}
	conn := newWebSocketConn(ws)
	defer conn.Close()

	handlerChan <- 1
	defer func() {
		handlerChan <- -1
	}()

	// Pass the address of client as the remote address of incoming connection
	clientIPParam := ws.Request().URL.Query().Get("client_ip")
	addr := clientAddr(clientIPParam)
	if addr == "" {
		statsChannel <- false
	} else {
		statsChannel <- true
	}
	or, err := pt.DialOr(&ptInfo, addr, ptMethodName)

	if err != nil {
		log.Printf("failed to connect to ORPort: %s", err)
		return
	}
	defer or.Close()

	proxy(or, &conn)
}

func initServer(addr *net.TCPAddr,
	getCertificate func(*tls.ClientHelloInfo) (*tls.Certificate, error),
	listenAndServe func(*http.Server, chan<- error)) (*http.Server, error) {
	// We're not capable of listening on port 0 (i.e., an ephemeral port
	// unknown in advance). The reason is that while the net/http package
	// exposes ListenAndServe and ListenAndServeTLS, those functions never
	// return, so there's no opportunity to find out what the port number
	// is, in between the Listen and Serve steps.
	// https://groups.google.com/d/msg/Golang-nuts/3F1VRCCENp8/3hcayZiwYM8J
	if addr.Port == 0 {
		return nil, fmt.Errorf("cannot listen on port %d; configure a port using ServerTransportListenAddr", addr.Port)
	}

	var config websocket.Config
	config.MaxMessageSize = maxMessageSize
	server := &http.Server{
		Addr:        addr.String(),
		Handler:     config.Handler(webSocketHandler),
		ReadTimeout: requestTimeout,
	}
	// We need to override server.TLSConfig.GetCertificate--but first
	// server.TLSConfig needs to be non-nil. If we just create our own new
	// &tls.Config, it will lack the default settings that the net/http
	// package sets up for things like HTTP/2. Therefore we first call
	// http2.ConfigureServer for its side effect of initializing
	// server.TLSConfig properly. An alternative would be to make a dummy
	// net.Listener, call Serve on it, and let it return.
	// https://github.com/golang/go/issues/16588#issuecomment-237386446
	err := http2.ConfigureServer(server, nil)
	if err != nil {
		return server, err
	}
	server.TLSConfig.GetCertificate = getCertificate

	// Another unfortunate effect of the inseparable net/http ListenAndServe
	// is that we can't check for Listen errors like "permission denied" and
	// "address already in use" without potentially entering the infinite
	// loop of Serve. The hack we apply here is to wait a short time,
	// listenAndServeErrorTimeout, to see if an error is returned (because
	// it's better if the error message goes to the tor log through
	// SMETHOD-ERROR than if it only goes to the snowflake log).
	errChan := make(chan error)
	go listenAndServe(server, errChan)
	select {
	case err = <-errChan:
		break
	case <-time.After(listenAndServeErrorTimeout):
		break
	}

	return server, err
}

func startServer(addr *net.TCPAddr) (*http.Server, error) {
	return initServer(addr, nil, func(server *http.Server, errChan chan<- error) {
		log.Printf("listening with plain HTTP on %s", addr)
		err := server.ListenAndServe()
		if err != nil {
			log.Printf("error in ListenAndServe: %s", err)
		}
		errChan <- err
	})
}

func startServerTLS(addr *net.TCPAddr, getCertificate func(*tls.ClientHelloInfo) (*tls.Certificate, error)) (*http.Server, error) {
	return initServer(addr, getCertificate, func(server *http.Server, errChan chan<- error) {
		log.Printf("listening with HTTPS on %s", addr)
		err := server.ListenAndServeTLS("", "")
		if err != nil {
			log.Printf("error in ListenAndServeTLS: %s", err)
		}
		errChan <- err
	})
}

func getCertificateCacheDir() (string, error) {
	stateDir, err := pt.MakeStateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(stateDir, "snowflake-certificate-cache"), nil
}

func main() {
	var acmeEmail string
	var acmeHostnamesCommas string
	var disableTLS bool
	var logFilename string

	flag.Usage = usage
	flag.StringVar(&acmeEmail, "acme-email", "", "optional contact email for Let's Encrypt notifications")
	flag.StringVar(&acmeHostnamesCommas, "acme-hostnames", "", "comma-separated hostnames for TLS certificate")
	flag.BoolVar(&disableTLS, "disable-tls", false, "don't use HTTPS")
	flag.StringVar(&logFilename, "log", "", "log file to write to")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.LUTC)

	var logOutput io.Writer = os.Stderr
	if logFilename != "" {
		f, err := os.OpenFile(logFilename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			log.Fatalf("can't open log file: %s", err)
		}
		defer f.Close()
		logOutput = f
	}
	//We want to send the log output through our scrubber first
	log.SetOutput(&safelog.LogScrubber{Output: logOutput})

	if !disableTLS && acmeHostnamesCommas == "" {
		log.Fatal("the --acme-hostnames option is required")
	}
	acmeHostnames := strings.Split(acmeHostnamesCommas, ",")

	log.Printf("starting")
	var err error
	ptInfo, err = pt.ServerSetup(nil)
	if err != nil {
		log.Fatalf("error in setup: %s", err)
	}

	go statsThread()

	var certManager *autocert.Manager
	if !disableTLS {
		log.Printf("ACME hostnames: %q", acmeHostnames)

		var cache autocert.Cache
		var cacheDir string
		cacheDir, err = getCertificateCacheDir()
		if err == nil {
			log.Printf("caching ACME certificates in directory %q", cacheDir)
			cache = autocert.DirCache(cacheDir)
		} else {
			log.Printf("disabling ACME certificate cache: %s", err)
		}

		certManager = &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(acmeHostnames...),
			Email:      acmeEmail,
			Cache:      cache,
		}
	}

	// The ACME HTTP-01 responder only works when it is running on port 80.
	// We actually open the port in the loop below, so that any errors can
	// be reported in the SMETHOD-ERROR of some bindaddr.
	// https://github.com/ietf-wg-acme/acme/blob/master/draft-ietf-acme-acme.md#http-challenge
	needHTTP01Listener := !disableTLS

	servers := make([]*http.Server, 0)
	for _, bindaddr := range ptInfo.Bindaddrs {
		if bindaddr.MethodName != ptMethodName {
			if err = pt.SmethodError(bindaddr.MethodName, "no such method"); err != nil {
				log.Printf("pt.SmethodError returned error: %v", err)
			}
			continue
		}

		if needHTTP01Listener {
			addr := *bindaddr.Addr
			addr.Port = 80
			log.Printf("Starting HTTP-01 ACME listener")
			var lnHTTP01 *net.TCPListener
			lnHTTP01, err = net.ListenTCP("tcp", &addr)
			if err != nil {
				log.Printf("error opening HTTP-01 ACME listener: %s", err)
				if inerr := pt.SmethodError(bindaddr.MethodName, "HTTP-01 ACME listener: "+err.Error()); inerr != nil {
					log.Printf("pt.SmethodError returned error: %v", inerr)
				}
				continue
			}
			server := &http.Server{
				Addr:    addr.String(),
				Handler: certManager.HTTPHandler(nil),
			}
			go func() {
				log.Fatal(server.Serve(lnHTTP01))
			}()
			servers = append(servers, server)
			needHTTP01Listener = false
		}

		var server *http.Server
		args := pt.Args{}
		if disableTLS {
			args.Add("tls", "no")
			server, err = startServer(bindaddr.Addr)
		} else {
			args.Add("tls", "yes")
			for _, hostname := range acmeHostnames {
				args.Add("hostname", hostname)
			}
			server, err = startServerTLS(bindaddr.Addr, certManager.GetCertificate)
		}
		if err != nil {
			log.Printf("error opening listener: %s", err)
			if inerr := pt.SmethodError(bindaddr.MethodName, err.Error()); inerr != nil {
				log.Printf("pt.SmethodError returned error: %v", inerr)
			}
			continue
		}
		pt.SmethodArgs(bindaddr.MethodName, bindaddr.Addr, args)
		servers = append(servers, server)
	}
	pt.SmethodsDone()

	var numHandlers int = 0
	var sig os.Signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM)

	if os.Getenv("TOR_PT_EXIT_ON_STDIN_CLOSE") == "1" {
		// This environment variable means we should treat EOF on stdin
		// just like SIGTERM: https://bugs.torproject.org/15435.
		go func() {
			if _, err := io.Copy(ioutil.Discard, os.Stdin); err != nil {
				log.Printf("error copying os.Stdin to ioutil.Discard: %v", err)
			}
			log.Printf("synthesizing SIGTERM because of stdin close")
			sigChan <- syscall.SIGTERM
		}()
	}

	// keep track of handlers and wait for a signal
	sig = nil
	for sig == nil {
		select {
		case n := <-handlerChan:
			numHandlers += n
		case sig = <-sigChan:
		}
	}

	// signal received, shut down
	log.Printf("caught signal %q, exiting", sig)
	for _, server := range servers {
		server.Close()
	}
	for numHandlers > 0 {
		numHandlers += <-handlerChan
	}
}
