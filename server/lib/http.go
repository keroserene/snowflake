package snowflake_server

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/encapsulation"
	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/turbotunnel"
	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/websocketconn"
	"github.com/gorilla/websocket"
)

const requestTimeout = 10 * time.Second

// How long to remember outgoing packets for a client, when we don't currently
// have an active WebSocket connection corresponding to that client. Because a
// client session may span multiple WebSocket connections, we keep packets we
// aren't able to send immediately in memory, for a little while but not
// indefinitely.
const clientMapTimeout = 1 * time.Minute

// How big to make the map of ClientIDs to IP addresses. The map is used in
// turbotunnelMode to store a reasonable IP address for a client session that
// may outlive any single WebSocket connection.
const clientIDAddrMapCapacity = 10240

// How long to wait for ListenAndServe or ListenAndServeTLS to return an error
// before deciding that it's not going to return.
const listenAndServeErrorTimeout = 100 * time.Millisecond

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// clientIDAddrMap stores short-term mappings from ClientIDs to IP addresses.
// When we call pt.DialOr, tor wants us to provide a USERADDR string that
// represents the remote IP address of the client (for metrics purposes, etc.).
// This data structure bridges the gap between ServeHTTP, which knows about IP
// addresses, and handleStream, which is what calls pt.DialOr. The common piece
// of information linking both ends of the chain is the ClientID, which is
// attached to the WebSocket connection and every session.
var clientIDAddrMap = newClientIDMap(clientIDAddrMapCapacity)

type httpHandler struct {
	// pconn is the adapter layer between stream-oriented WebSocket
	// connections and the packet-oriented KCP layer.
	pconn *turbotunnel.QueuePacketConn
}

func (handler *httpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
		// Close the conn as we no longer support the old
		// one-session-per-WebSocket mode.
		log.Println("Received unsupported oneshot connection")
		return
	}
	if err != nil {
		log.Println(err)
		return
	}
}

// turbotunnelMode handles clients that sent turbotunnel.Token at the start of
// their stream. These clients expect to send and receive encapsulated packets,
// with a long-lived session identified by ClientID.
func turbotunnelMode(conn net.Conn, addr net.Addr, pconn *turbotunnel.QueuePacketConn) error {
	// Read the ClientID prefix. Every packet encapsulated in this WebSocket
	// connection pertains to the same ClientID.
	var clientID turbotunnel.ClientID
	_, err := io.ReadFull(conn, clientID[:])
	if err != nil {
		return fmt.Errorf("reading ClientID: %v", err)
	}

	// Store a a short-term mapping from the ClientID to the client IP
	// address attached to this WebSocket connection. tor will want us to
	// provide a client IP address when we call pt.DialOr. But a KCP session
	// does not necessarily correspond to any single IP address--it's
	// composed of packets that are carried in possibly multiple WebSocket
	// streams. We apply the heuristic that the IP address of the most
	// recent WebSocket connection that has had to do with a session, at the
	// time the session is established, is the IP address that should be
	// credited for the entire KCP session.
	clientIDAddrMap.Set(clientID, addr)

	var wg sync.WaitGroup
	wg.Add(2)
	done := make(chan struct{})

	// The remainder of the WebSocket stream consists of encapsulated
	// packets. We read them one by one and feed them into the
	// QueuePacketConn on which kcp.ServeConn was set up, which eventually
	// leads to KCP-level sessions in the acceptSessions function.
	go func() {
		defer wg.Done()
		defer close(done) // Signal the write loop to finish
		for {
			p, err := encapsulation.ReadData(conn)
			if err != nil {
				return
			}
			pconn.QueueIncoming(p, clientID)
		}
	}()

	// At the same time, grab packets addressed to this ClientID and
	// encapsulate them into the downstream.
	go func() {
		defer wg.Done()
		defer conn.Close() // Signal the read loop to finish

		// Buffer encapsulation.WriteData operations to keep length
		// prefixes in the same send as the data that follows.
		bw := bufio.NewWriter(conn)
		for {
			select {
			case <-done:
				return
			case p, ok := <-pconn.OutgoingQueue(clientID):
				if !ok {
					return
				}
				_, err := encapsulation.WriteData(bw, p)
				if err == nil {
					err = bw.Flush()
				}
				if err != nil {
					return
				}
			}
		}
	}()

	wg.Wait()

	return nil
}

// ClientMapAddr is a string that represents a connecting client.
type ClientMapAddr string

func (addr ClientMapAddr) Network() string {
	return "snowflake"
}

func (addr ClientMapAddr) String() string {
	return string(addr)
}

// Return a client address
func clientAddr(clientIPParam string) net.Addr {
	if clientIPParam == "" {
		return ClientMapAddr("")
	}
	// Check if client addr is a valid IP
	clientIP := net.ParseIP(clientIPParam)
	if clientIP == nil {
		return ClientMapAddr("")
	}
	// Check if client addr is 0.0.0.0 or [::]. Some proxies erroneously
	// report an address of 0.0.0.0: https://bugs.torproject.org/33157.
	if clientIP.IsUnspecified() {
		return ClientMapAddr("")
	}
	// Add a stub port number. USERADDR requires a port number.
	return ClientMapAddr((&net.TCPAddr{IP: clientIP, Port: 1, Zone: ""}).String())
}
