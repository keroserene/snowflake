// Snowflake-specific websocket server plugin. This is the same as the websocket
// server used by flash proxy, except that it reports the transport name as
// "snowflake" and does not forward the remote address to the ExtORPort.
//
// Usage in torrc:
// 	ExtORPort auto
// 	ServerTransportPlugin snowflake exec server --port 9902
package main

import (
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"git.torproject.org/pluggable-transports/goptlib.git"
	"git.torproject.org/pluggable-transports/websocket.git/websocket"
)

const ptMethodName = "snowflake"
const requestTimeout = 10 * time.Second

// "4/3+1" accounts for possible base64 encoding.
const maxMessageSize = 64*1024*4/3 + 1

var logFile = os.Stderr

var ptInfo pt.ServerInfo

// When a connection handler starts, +1 is written to this channel; when it
// ends, -1 is written.
var handlerChan = make(chan int)

func usage() {
	fmt.Printf("Usage: %s [OPTIONS]\n", os.Args[0])
	fmt.Printf("WebSocket server pluggable transport for Tor.\n")
	fmt.Printf("Works only as a managed proxy.\n")
	fmt.Printf("\n")
	fmt.Printf("  -h, --help   show this help.\n")
	fmt.Printf("  --log FILE   log messages to FILE (default stderr).\n")
	fmt.Printf("  --port PORT  listen on PORT (overrides Tor's requested port).\n")
}

// An abstraction that makes an underlying WebSocket connection look like an
// io.ReadWriteCloser. It internally takes care of things like base64 encoding
// and decoding.
type webSocketConn struct {
	Ws         *websocket.WebSocket
	Base64     bool
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
		if conn.Base64 {
			if m.Opcode != 1 {
				err = errors.New(fmt.Sprintf("got non-text opcode %d with the base64 subprotocol", m.Opcode))
				return
			}
			conn.messageBuf = make([]byte, base64.StdEncoding.DecodedLen(len(m.Payload)))
			var num int
			num, err = base64.StdEncoding.Decode(conn.messageBuf, m.Payload)
			if err != nil {
				return
			}
			conn.messageBuf = conn.messageBuf[:num]
		} else {
			if m.Opcode != 2 {
				err = errors.New(fmt.Sprintf("got non-binary opcode %d with no subprotocol", m.Opcode))
				return
			}
			conn.messageBuf = m.Payload
		}
	}

	n = copy(b, conn.messageBuf)
	conn.messageBuf = conn.messageBuf[n:]

	return
}

// Implements io.Writer.
func (conn *webSocketConn) Write(b []byte) (n int, err error) {
	if conn.Base64 {
		buf := make([]byte, base64.StdEncoding.EncodedLen(len(b)))
		base64.StdEncoding.Encode(buf, b)
		err = conn.Ws.WriteMessage(1, buf)
		if err != nil {
			return
		}
		n = len(b)
	} else {
		err = conn.Ws.WriteMessage(2, b)
		n = len(b)
	}
	return
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
	conn.Base64 = (ws.Subprotocol == "base64")
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
		log.Printf("Failed to connect to ORPort: " + err.Error())
		return
	}
	defer or.Close()

	proxy(or, &conn)
}

func startListener(addr *net.TCPAddr) (*net.TCPListener, error) {
	ln, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return nil, err
	}
	go func() {
		defer ln.Close()
		var config websocket.Config
		config.Subprotocols = []string{"base64"}
		config.MaxMessageSize = maxMessageSize
		s := &http.Server{
			Handler:     config.Handler(webSocketHandler),
			ReadTimeout: requestTimeout,
		}
		err = s.Serve(ln)
		if err != nil {
			log.Printf("http.Serve: " + err.Error())
		}
	}()
	return ln, nil
}

func main() {
	var logFilename string
	var port int

	flag.Usage = usage
	flag.StringVar(&logFilename, "log", "", "log file to write to")
	flag.IntVar(&port, "port", 0, "port to listen on if unspecified by Tor")
	flag.Parse()

	if logFilename != "" {
		f, err := os.OpenFile(logFilename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Can't open log file %q: %s.\n", logFilename, err.Error())
			os.Exit(1)
		}
		log.SetOutput(f)
	}

	log.SetFlags(log.LstdFlags | log.LUTC)
	log.Printf("starting")
	var err error
	ptInfo, err = pt.ServerSetup(nil)
	if err != nil {
		log.Printf("error in setup: %s", err)
		os.Exit(1)
	}

	listeners := make([]*net.TCPListener, 0)
	for _, bindaddr := range ptInfo.Bindaddrs {
		// Override tor's requested port (which is 0 if this transport
		// has not been run before) with the one requested by the --port
		// option.
		if port != 0 {
			bindaddr.Addr.Port = port
		}

		switch bindaddr.MethodName {
		case ptMethodName:
			ln, err := startListener(bindaddr.Addr)
			if err != nil {
				pt.SmethodError(bindaddr.MethodName, err.Error())
				break
			}
			pt.Smethod(bindaddr.MethodName, ln.Addr())
			log.Printf("listening on %s", ln.Addr().String())
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
	log.Printf("Got first signal %q with %d running handlers.", sig, numHandlers)
	for _, ln := range listeners {
		ln.Close()
	}

	if sig == syscall.SIGTERM {
		log.Printf("Caught signal %q, exiting.", sig)
		return
	}

	// wait for second signal or no more handlers
	sig = nil
	for sig == nil && numHandlers != 0 {
		select {
		case n := <-handlerChan:
			numHandlers += n
			log.Printf("%d remaining handlers.", numHandlers)
		case sig = <-sigChan:
		}
	}
	if sig != nil {
		log.Printf("Got second signal %q with %d running handlers.", sig, numHandlers)
	}
}
