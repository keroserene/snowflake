// Snowflake-specific websocket server plugin. This is the same as the websocket
// server used by flash proxy, except that it reports the transport name as
// "snowflake" and does not forward the remote address to the ExtORPort.
//
// Usage in torrc:
// 	ExtORPort auto
// 	ServerTransportListenAddr snowflake 0.0.0.0:9902
// 	ServerTransportPlugin snowflake exec server
package main

import (
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"git.torproject.org/pluggable-transports/goptlib.git"
	"git.torproject.org/pluggable-transports/websocket.git/websocket"
	"golang.org/x/crypto/acme/autocert"
)

const ptMethodName = "snowflake"
const requestTimeout = 10 * time.Second

const maxMessageSize = 64 * 1024

var ptInfo pt.ServerInfo

// When a connection handler starts, +1 is written to this channel; when it
// ends, -1 is written.
var handlerChan = make(chan int)

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: %s [OPTIONS]

WebSocket server pluggable transport for Snowflake. Works only as a managed
proxy. Uses TLS with ACME (Let's Encrypt) by default. Set the certificate
hostnames with the --acme-hostnames option. Use ServerTransportListenAddr in
torrc to choose the listening port. When using TLS, if the port is not 443, this
program will open an additional listening port on 443 to work with ACME.

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
			err = errors.New(fmt.Sprintf("got non-binary opcode %d", m.Opcode))
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
		_, err := io.Copy(conn, local)
		if err != nil {
			log.Printf("error copying ORPort to WebSocket")
		}
		local.CloseRead()
		conn.Close()
		wg.Done()
	}()
	go func() {
		_, err := io.Copy(local, conn)
		if err != nil {
			log.Printf("error copying WebSocket to ORPort")
		}
		local.CloseWrite()
		conn.Close()
		wg.Done()
	}()

	wg.Wait()
}

func webSocketHandler(ws *websocket.WebSocket) {
	// Undo timeouts on HTTP request handling.
	ws.Conn.SetDeadline(time.Time{})
	conn := newWebSocketConn(ws)
	defer conn.Close()

	handlerChan <- 1
	defer func() {
		handlerChan <- -1
	}()

	// Pass an empty string for the client address. The remote address on
	// the incoming connection reflects that of the browser proxy, not of
	// the client. See https://bugs.torproject.org/18628.
	or, err := pt.DialOr(&ptInfo, "", ptMethodName)
	if err != nil {
		log.Printf("failed to connect to ORPort: %s", err)
		return
	}
	defer or.Close()

	proxy(or, &conn)
}

func listenTLS(network string, addr *net.TCPAddr, m *autocert.Manager) (net.Listener, error) {
	// This is cribbed from the source of net/http.Server.ListenAndServeTLS.
	// We have to separate the Listen and Serve parts because we need to
	// report the listening address before entering Serve (which is an
	// infinite loop).
	// https://groups.google.com/d/msg/Golang-nuts/3F1VRCCENp8/3hcayZiwYM8J
	config := &tls.Config{}
	config.NextProtos = []string{"http/1.1"}
	config.GetCertificate = m.GetCertificate

	conn, err := net.ListenTCP(network, addr)
	if err != nil {
		return nil, err
	}

	// Additionally disable SSLv3 because of the POODLE attack.
	// http://googleonlinesecurity.blogspot.com/2014/10/this-poodle-bites-exploiting-ssl-30.html
	// https://code.google.com/p/go/source/detail?r=ad9e191a51946e43f1abac8b6a2fefbf2291eea7
	config.MinVersion = tls.VersionTLS10

	tlsListener := tls.NewListener(conn, config)

	return tlsListener, nil
}

func startListener(network string, addr *net.TCPAddr) (net.Listener, error) {
	ln, err := net.ListenTCP(network, addr)
	if err != nil {
		return nil, err
	}
	log.Printf("listening with plain HTTP on %s", ln.Addr())
	return startServer(ln)
}

func startListenerTLS(network string, addr *net.TCPAddr, m *autocert.Manager) (net.Listener, error) {
	ln, err := listenTLS(network, addr, m)
	if err != nil {
		return nil, err
	}
	log.Printf("listening with HTTPS on %s", ln.Addr())
	return startServer(ln)
}

func startServer(ln net.Listener) (net.Listener, error) {
	go func() {
		defer ln.Close()
		var config websocket.Config
		config.MaxMessageSize = maxMessageSize
		s := &http.Server{
			Handler:     config.Handler(webSocketHandler),
			ReadTimeout: requestTimeout,
		}
		err := s.Serve(ln)
		if err != nil {
			log.Printf("http.Serve: %s", err)
		}
	}()
	return ln, nil
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
	if logFilename != "" {
		f, err := os.OpenFile(logFilename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			log.Fatalf("can't open log file: %s", err)
		}
		log.SetOutput(f)
	}

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

	if !disableTLS {
		log.Printf("ACME hostnames: %q", acmeHostnames)
	}
	certManager := autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(acmeHostnames...),
		Email:      acmeEmail,
	}

	// The ACME responder only works when it is running on port 443. In case
	// there is not already going to be a TLS listener on port 443, we need
	// to open an additional one. The port is actually opened in the loop
	// below, so that any errors can be reported in the SMETHOD-ERROR of
	// another bindaddr.
	// https://letsencrypt.github.io/acme-spec/#domain-validation-with-server-name-indication-dvsni
	need443Listener := !disableTLS
	for _, bindaddr := range ptInfo.Bindaddrs {
		if !disableTLS && bindaddr.Addr.Port == 443 {
			need443Listener = false
			break
		}
	}

	listeners := make([]net.Listener, 0)
	for _, bindaddr := range ptInfo.Bindaddrs {
		if bindaddr.MethodName != ptMethodName {
			pt.SmethodError(bindaddr.MethodName, "no such method")
			continue
		}

		if need443Listener {
			addr := *bindaddr.Addr
			addr.Port = 443
			log.Printf("opening additional ACME listener on %s", addr.String())
			ln443, err := startListenerTLS("tcp", &addr, &certManager)
			if err != nil {
				log.Printf("error opening ACME listener: %s", err)
				pt.SmethodError(bindaddr.MethodName, "ACME listener: "+err.Error())
				continue
			}
			listeners = append(listeners, ln443)
			need443Listener = false
		}

		var ln net.Listener
		args := pt.Args{}
		if disableTLS {
			args.Add("tls", "no")
			ln, err = startListener("tcp", bindaddr.Addr)
		} else {
			args.Add("tls", "yes")
			ln, err = startListenerTLS("tcp", bindaddr.Addr, &certManager)
		}
		if err != nil {
			log.Printf("error opening listener: %s", err)
			pt.SmethodError(bindaddr.MethodName, err.Error())
			continue
		}
		pt.SmethodArgs(bindaddr.MethodName, ln.Addr(), args)
		listeners = append(listeners, ln)
	}
	pt.SmethodsDone()

	var numHandlers int = 0
	var sig os.Signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM)

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
	for _, ln := range listeners {
		ln.Close()
	}
	for n := range handlerChan {
		numHandlers += n
		if numHandlers == 0 {
			break
		}
	}
}
