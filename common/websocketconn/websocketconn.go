package websocketconn

import (
	"io"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// An abstraction that makes an underlying WebSocket connection look like an
// io.ReadWriteCloser.
type WebSocketConn struct {
	Ws *websocket.Conn
	r  io.Reader
}

// Implements io.Reader.
func (conn *WebSocketConn) Read(b []byte) (n int, err error) {
	var opCode int
	if conn.r == nil {
		// New message
		var r io.Reader
		for {
			if opCode, r, err = conn.Ws.NextReader(); err != nil {
				return
			}
			if opCode != websocket.BinaryMessage && opCode != websocket.TextMessage {
				continue
			}

			conn.r = r
			break
		}
	}

	n, err = conn.r.Read(b)
	if err == io.EOF {
		// Message finished
		conn.r = nil
		err = nil
	}
	return
}

// Implements io.Writer.
func (conn *WebSocketConn) Write(b []byte) (n int, err error) {
	var w io.WriteCloser
	if w, err = conn.Ws.NextWriter(websocket.BinaryMessage); err != nil {
		return
	}
	if n, err = w.Write(b); err != nil {
		return
	}
	err = w.Close()
	return
}

// Implements io.Closer.
func (conn *WebSocketConn) Close() error {
	// Ignore any error in trying to write a Close frame.
	_ = conn.Ws.WriteControl(websocket.CloseMessage, []byte{}, time.Now().Add(time.Second))
	return conn.Ws.Close()
}

// Create a new WebSocketConn.
func NewWebSocketConn(ws *websocket.Conn) WebSocketConn {
	var conn WebSocketConn
	conn.Ws = ws
	return conn
}

// Copy from WebSocket to socket and vice versa.
func CopyLoop(c1 io.ReadWriteCloser, c2 io.ReadWriteCloser) {
	var wg sync.WaitGroup
	copyer := func(dst io.ReadWriteCloser, src io.ReadWriteCloser) {
		defer wg.Done()
		if _, err := io.Copy(dst, src); err != nil {
			log.Printf("io.Copy inside CopyLoop generated an error: %v", err)
		}
		dst.Close()
		src.Close()
	}
	wg.Add(2)
	go copyer(c1, c2)
	go copyer(c2, c1)
	wg.Wait()
}
