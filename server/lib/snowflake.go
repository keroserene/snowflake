package lib

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"git.torproject.org/pluggable-transports/snowflake.git/common/turbotunnel"
	"github.com/xtaci/kcp-go/v5"
	"github.com/xtaci/smux"
	"golang.org/x/net/http2"
)

// Transport is a structure with methods that conform to the Go PT v2.1 API
// https://github.com/Pluggable-Transports/Pluggable-Transports-spec/blob/master/releases/PTSpecV2.1/Pluggable%20Transport%20Specification%20v2.1%20-%20Go%20Transport%20API.pdf
type Transport struct {
	getCertificate func(*tls.ClientHelloInfo) (*tls.Certificate, error)
}

func NewSnowflakeServer(getCertificate func(*tls.ClientHelloInfo) (*tls.Certificate, error)) *Transport {

	return &Transport{getCertificate: getCertificate}
}

func (t *Transport) Listen(addr net.Addr) (*SnowflakeListener, error) {
	listener := &SnowflakeListener{addr: addr, queue: make(chan net.Conn, 65534)}

	handler := HTTPHandler{
		// pconn is shared among all connections to this server. It
		// overlays packet-based client sessions on top of ephemeral
		// WebSocket connections.
		pconn: turbotunnel.NewQueuePacketConn(addr, clientMapTimeout),
	}
	server := &http.Server{
		Addr:        addr.String(),
		Handler:     &handler,
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
		return nil, err
	}
	server.TLSConfig.GetCertificate = t.getCertificate

	// Another unfortunate effect of the inseparable net/http ListenAndServe
	// is that we can't check for Listen errors like "permission denied" and
	// "address already in use" without potentially entering the infinite
	// loop of Serve. The hack we apply here is to wait a short time,
	// listenAndServeErrorTimeout, to see if an error is returned (because
	// it's better if the error message goes to the tor log through
	// SMETHOD-ERROR than if it only goes to the snowflake log).
	errChan := make(chan error)
	go func() {
		if t.getCertificate == nil {
			// TLS is disabled
			log.Printf("listening with plain HTTP on %s", addr)
			err := server.ListenAndServe()
			if err != nil {
				log.Printf("error in ListenAndServe: %s", err)
			}
			errChan <- err
		} else {
			log.Printf("listening with HTTPS on %s", addr)
			err := server.ListenAndServeTLS("", "")
			if err != nil {
				log.Printf("error in ListenAndServeTLS: %s", err)
			}
			errChan <- err
		}
	}()

	select {
	case err = <-errChan:
		break
	case <-time.After(listenAndServeErrorTimeout):
		break
	}

	listener.server = server

	// Start a KCP engine, set up to read and write its packets over the
	// WebSocket connections that arrive at the web server.
	// handler.ServeHTTP is responsible for encapsulation/decapsulation of
	// packets on behalf of KCP. KCP takes those packets and turns them into
	// sessions which appear in the acceptSessions function.
	ln, err := kcp.ServeConn(nil, 0, 0, handler.pconn)
	if err != nil {
		server.Close()
		return nil, err
	}
	go func() {
		defer ln.Close()
		err := listener.acceptSessions(ln)
		if err != nil {
			log.Printf("acceptSessions: %v", err)
		}
	}()

	listener.ln = ln

	return listener, nil

}

type SnowflakeListener struct {
	addr      net.Addr
	queue     chan net.Conn
	server    *http.Server
	ln        *kcp.Listener
	closed    chan struct{}
	closeOnce sync.Once
}

// Allows the caller to accept incoming Snowflake connections
// We accept connections from a queue to accommodate both incoming
// smux Streams and legacy non-turbotunnel connections
func (l *SnowflakeListener) Accept() (net.Conn, error) {
	select {
	case <-l.closed:
		//channel has been closed, no longer accepting connections
		return nil, io.ErrClosedPipe
	case conn := <-l.queue:
		return conn, nil
	}
}

func (l *SnowflakeListener) Addr() net.Addr {
	return l.addr
}

func (l *SnowflakeListener) Close() error {
	// Close our HTTP server and our KCP listener
	l.closeOnce.Do(func() {
		close(l.closed)
		l.server.Close()
		l.ln.Close()
	})
	return nil
}

// acceptStreams layers an smux.Session on the KCP connection and awaits streams
// on it. Passes each stream to our SnowflakeListener accept queue.
func (l *SnowflakeListener) acceptStreams(conn *kcp.UDPSession) error {
	// Look up the IP address associated with this KCP session, via the
	// ClientID that is returned by the session's RemoteAddr method.
	addr, ok := clientIDAddrMap.Get(conn.RemoteAddr().(turbotunnel.ClientID))
	if !ok {
		// This means that the map is tending to run over capacity, not
		// just that there was not client_ip on the incoming connection.
		// We store "" in the map in the absence of client_ip. This log
		// message means you should increase clientIDAddrMapCapacity.
		log.Printf("no address in clientID-to-IP map (capacity %d)", clientIDAddrMapCapacity)
	}

	smuxConfig := smux.DefaultConfig()
	smuxConfig.Version = 2
	smuxConfig.KeepAliveTimeout = 10 * time.Minute
	sess, err := smux.Server(conn, smuxConfig)
	if err != nil {
		return err
	}

	for {
		stream, err := sess.AcceptStream()
		if err != nil {
			if err, ok := err.(net.Error); ok && err.Temporary() {
				continue
			}
			return err
		}
		l.QueueConn(&SnowflakeClientConn{Conn: stream, address: clientAddr(addr)})
	}
}

// acceptSessions listens for incoming KCP connections and passes them to
// acceptStreams. It is handler.ServeHTTP that provides the network interface
// that drives this function.
func (l *SnowflakeListener) acceptSessions(ln *kcp.Listener) error {
	for {
		conn, err := ln.AcceptKCP()
		if err != nil {
			if err, ok := err.(net.Error); ok && err.Temporary() {
				continue
			}
			return err
		}
		// Permit coalescing the payloads of consecutive sends.
		conn.SetStreamMode(true)
		// Set the maximum send and receive window sizes to a high number
		// Removes KCP bottlenecks: https://gitlab.torproject.org/tpo/anti-censorship/pluggable-transports/snowflake/-/issues/40026
		conn.SetWindowSize(65535, 65535)
		// Disable the dynamic congestion window (limit only by the
		// maximum of local and remote static windows).
		conn.SetNoDelay(
			0, // default nodelay
			0, // default interval
			0, // default resend
			1, // nc=1 => congestion window off
		)
		go func() {
			defer conn.Close()
			err := l.acceptStreams(conn)
			if err != nil && err != io.ErrClosedPipe {
				log.Printf("acceptStreams: %v", err)
			}
		}()
	}
}

func (l *SnowflakeListener) QueueConn(conn net.Conn) error {
	select {
	case <-l.closed:
		return fmt.Errorf("accepted connection on closed listener")
	case l.queue <- conn:
		return nil
	}
}

// A wrapper for the underlying oneshot or turbotunnel conn
// because we need to reference our mapping to determine the client
// address
type SnowflakeClientConn struct {
	net.Conn
	address net.Addr
}

func (conn *SnowflakeClientConn) RemoteAddr() net.Addr {
	return conn.address
}
