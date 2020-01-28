package websocketconn

import (
	"io"
	"time"

	"github.com/gorilla/websocket"
)

// An abstraction that makes an underlying WebSocket connection look like an
// io.ReadWriteCloser.
type Conn struct {
	Ws *websocket.Conn
	r  io.Reader
}

// Implements io.Reader.
func (conn *Conn) Read(b []byte) (n int, err error) {
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
func (conn *Conn) Write(b []byte) (n int, err error) {
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
func (conn *Conn) Close() error {
	// Ignore any error in trying to write a Close frame.
	_ = conn.Ws.WriteControl(websocket.CloseMessage, []byte{}, time.Now().Add(time.Second))
	return conn.Ws.Close()
}

// Create a new Conn.
func New(ws *websocket.Conn) *Conn {
	var conn Conn
	conn.Ws = ws
	return &conn
}
