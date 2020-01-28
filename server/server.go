// Snowflake-specific websocket server plugin. It reports the transport name as
// "snowflake".
package main

import (
	"bufio"
	"bytes"
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

	pt "git.torproject.org/pluggable-transports/goptlib.git"
	"git.torproject.org/pluggable-transports/snowflake.git/common/encapsulation"
	"git.torproject.org/pluggable-transports/snowflake.git/common/safelog"
	"git.torproject.org/pluggable-transports/snowflake.git/common/turbotunnel"
	"git.torproject.org/pluggable-transports/snowflake.git/common/websocketconn"
	"github.com/gorilla/websocket"
	"github.com/xtaci/kcp-go/v5"
	"github.com/xtaci/smux"
	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/net/http2"
)

const ptMethodName = "snowflake"
const requestTimeout = 10 * time.Second

// How long to remember outgoing packets for a client, when we don't currently
// have an active WebSocket connection corresponding to that client. Because a
// client session may span multiple WebSocket connections, we keep packets we
// aren't able to send immediately in memory, for a little while but not
// indefinitely.
const clientMapTimeout = 1 * time.Minute

// How long to wait for ListenAndServe or ListenAndServeTLS to return an error
// before deciding that it's not going to return.
const listenAndServeErrorTimeout = 100 * time.Millisecond

var ptInfo pt.ServerInfo

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

// Copy from one stream to another.
func proxy(local *net.TCPConn, conn net.Conn) {
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
			log.Printf("error copying WebSocket to ORPort %v", err)
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
	// Check if client addr is 0.0.0.0 or [::]. Some proxies erroneously
	// report an address of 0.0.0.0: https://bugs.torproject.org/33157.
	if clientIP.IsUnspecified() {
		return ""
	}
	// Add a dummy port number. USERADDR requires a port number.
	return (&net.TCPAddr{IP: clientIP, Port: 1, Zone: ""}).String()
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// overrideReadConn is a net.Conn with an overridden Read method. Compare to
// recordingConn at
// https://dave.cheney.net/2015/05/22/struct-composition-with-go.
type overrideReadConn struct {
	net.Conn
	io.Reader
}

func (conn *overrideReadConn) Read(p []byte) (int, error) {
	return conn.Reader.Read(p)
}

type HTTPHandler struct {
	// pconn is the adapter layer between stream-oriented WebSocket
	// connections and the packet-oriented KCP layer.
	pconn *turbotunnel.QueuePacketConn
}

func (handler *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	conn := websocketconn.New(ws)
	defer conn.Close()

	// Pass the address of client as the remote address of incoming connection
	clientIPParam := r.URL.Query().Get("client_ip")
	addr := clientAddr(clientIPParam)

	var token [len(turbotunnel.Token)]byte
	_, err = io.ReadFull(conn, token[:])
	if err != nil {
		// Don't bother logging EOF: that happens with an unused
		// connection, which clients make frequently as they maintain a
		// pool of proxies.
		if err != io.EOF {
			log.Printf("reading token: %v", err)
		}
		return
	}

	switch {
	case bytes.Equal(token[:], turbotunnel.Token[:]):
		err = turbotunnelMode(conn, addr, handler.pconn)
	default:
		// We didn't find a matching token, which means that we are
		// dealing with a client that doesn't know about such things.
		// "Unread" the token by constructing a new Reader and pass it
		// to the old one-session-per-WebSocket mode.
		conn2 := &overrideReadConn{Conn: conn, Reader: io.MultiReader(bytes.NewReader(token[:]), conn)}
		err = oneshotMode(conn2, addr)
	}
	if err != nil {
		log.Println(err)
		return
	}
}

// oneshotMode handles clients that did not send turbotunnel.Token at the start
// of their stream. These clients use the WebSocket as a raw pipe, and expect
// their session to begin and end when this single WebSocket does.
func oneshotMode(conn net.Conn, addr string) error {
	statsChannel <- addr != ""
	or, err := pt.DialOr(&ptInfo, addr, ptMethodName)
	if err != nil {
		return fmt.Errorf("failed to connect to ORPort: %s", err)
	}
	defer or.Close()

	proxy(or, conn)

	return nil
}

// turbotunnelMode handles clients that sent turbotunnel.Token at the start of
// their stream. These clients expect to send and receive encapsulated packets,
// with a long-lived session identified by ClientID.
func turbotunnelMode(conn net.Conn, addr string, pconn *turbotunnel.QueuePacketConn) error {
	// Read the ClientID prefix. Every packet encapsulated in this WebSocket
	// connection pertains to the same ClientID.
	var clientID turbotunnel.ClientID
	_, err := io.ReadFull(conn, clientID[:])
	if err != nil {
		return fmt.Errorf("reading ClientID: %v", err)
	}

	// TODO: ClientID-to-client_ip address mapping
	// Peek at the first read packet to get the KCP conv ID.

	errCh := make(chan error)

	// The remainder of the WebSocket stream consists of encapsulated
	// packets. We read them one by one and feed them into the
	// QueuePacketConn on which kcp.ServeConn was set up, which eventually
	// leads to KCP-level sessions in the acceptSessions function.
	go func() {
		for {
			p, err := encapsulation.ReadData(conn)
			if err != nil {
				errCh <- err
				break
			}
			pconn.QueueIncoming(p, clientID)
		}
	}()

	// At the same time, grab packets addressed to this ClientID and
	// encapsulate them into the downstream.
	go func() {
		// Buffer encapsulation.WriteData operations to keep length
		// prefixes in the same send as the data that follows.
		bw := bufio.NewWriter(conn)
		for p := range pconn.OutgoingQueue(clientID) {
			_, err := encapsulation.WriteData(bw, p)
			if err == nil {
				err = bw.Flush()
			}
			if err != nil {
				errCh <- err
				break
			}
		}
	}()

	// Wait until one of the above loops terminates. The closing of the
	// WebSocket connection will terminate the other one.
	<-errCh

	return nil
}

// handleStream bidirectionally connects a client stream with the ORPort.
func handleStream(stream net.Conn) error {
	// TODO: This is where we need to provide the client IP address.
	statsChannel <- false
	or, err := pt.DialOr(&ptInfo, "", ptMethodName)
	if err != nil {
		return fmt.Errorf("connecting to ORPort: %v", err)
	}
	defer or.Close()

	proxy(or, stream)

	return nil
}

// acceptStreams layers an smux.Session on the KCP connection and awaits streams
// on it. Passes each stream to handleStream.
func acceptStreams(conn *kcp.UDPSession) error {
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
		go func() {
			defer stream.Close()
			err := handleStream(stream)
			if err != nil {
				log.Printf("handleStream: %v", err)
			}
		}()
	}
}

// acceptSessions listens for incoming KCP connections and passes them to
// acceptStreams. It is handler.ServeHTTP that provides the network interface
// that drives this function.
func acceptSessions(ln *kcp.Listener) error {
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
			err := acceptStreams(conn)
			if err != nil {
				log.Printf("acceptStreams: %v", err)
			}
		}()
	}
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

	// Start a KCP engine, set up to read and write its packets over the
	// WebSocket connections that arrive at the web server.
	// handler.ServeHTTP is responsible for encapsulation/decapsulation of
	// packets on behalf of KCP. KCP takes those packets and turns them into
	// sessions which appear in the acceptSessions function.
	ln, err := kcp.ServeConn(nil, 0, 0, handler.pconn)
	if err != nil {
		server.Close()
		return server, err
	}
	go func() {
		defer ln.Close()
		err := acceptSessions(ln)
		if err != nil {
			log.Printf("acceptSessions: %v", err)
		}
	}()

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
	var unsafeLogging bool

	flag.Usage = usage
	flag.StringVar(&acmeEmail, "acme-email", "", "optional contact email for Let's Encrypt notifications")
	flag.StringVar(&acmeHostnamesCommas, "acme-hostnames", "", "comma-separated hostnames for TLS certificate")
	flag.BoolVar(&disableTLS, "disable-tls", false, "don't use HTTPS")
	flag.StringVar(&logFilename, "log", "", "log file to write to")
	flag.BoolVar(&unsafeLogging, "unsafe-logging", false, "prevent logs from being scrubbed")
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
	if unsafeLogging {
		log.SetOutput(logOutput)
	} else {
		// We want to send the log output through our scrubber first
		log.SetOutput(&safelog.LogScrubber{Output: logOutput})
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
			pt.SmethodError(bindaddr.MethodName, "no such method")
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
				pt.SmethodError(bindaddr.MethodName, "HTTP-01 ACME listener: "+err.Error())
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
			pt.SmethodError(bindaddr.MethodName, err.Error())
			continue
		}
		pt.SmethodArgs(bindaddr.MethodName, bindaddr.Addr, args)
		servers = append(servers, server)
	}
	pt.SmethodsDone()

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

	// Wait for a signal.
	sig := <-sigChan

	// Signal received, shut down.
	log.Printf("caught signal %q, exiting", sig)
	for _, server := range servers {
		server.Close()
	}
}
