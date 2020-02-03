package websocketconn

import (
	"io"
	"time"

	"github.com/gorilla/websocket"
)

// An abstraction that makes an underlying WebSocket connection look like an
// io.ReadWriteCloser.
type Conn struct {
	ws     *websocket.Conn
	Reader io.Reader
	Writer io.Writer
}

// Implements io.Reader.
func (conn *Conn) Read(b []byte) (n int, err error) {
	return conn.Reader.Read(b)
}

// Implements io.Writer.
func (conn *Conn) Write(b []byte) (n int, err error) {
	return conn.Writer.Write(b)
}

// Implements io.Closer.
func (conn *Conn) Close() error {
	// Ignore any error in trying to write a Close frame.
	_ = conn.ws.WriteControl(websocket.CloseMessage, []byte{}, time.Now().Add(time.Second))
	return conn.ws.Close()
}

func readLoop(w io.Writer, ws *websocket.Conn) error {
	for {
		messageType, r, err := ws.NextReader()
		if err != nil {
			return err
		}
		if messageType != websocket.BinaryMessage && messageType != websocket.TextMessage {
			continue
		}
		_, err = io.Copy(w, r)
		if err != nil {
			return err
		}
	}
	return nil
}

func writeLoop(ws *websocket.Conn, r io.Reader) error {
	for {
		var buf [2048]byte
		n, err := r.Read(buf[:])
		if err != nil {
			return err
		}
		data := buf[:n]
		w, err := ws.NextWriter(websocket.BinaryMessage)
		if err != nil {
			return err
		}
		n, err = w.Write(data)
		if err != nil {
			return err
		}
		err = w.Close()
		if err != nil {
			return err
		}
	}
}

// websocket.Conn methods start returning websocket.CloseError after the
// connection has been closed. We want to instead interpret that as io.EOF, just
// as you would find with a normal net.Conn. This only converts
// websocket.CloseErrors with known codes; other codes like CloseProtocolError
// and CloseAbnormalClosure will still be reported as anomalous.
func closeErrorToEOF(err error) error {
	if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseNoStatusReceived) {
		err = io.EOF
	}
	return err
}

// Create a new Conn.
func New(ws *websocket.Conn) *Conn {
	// Set up synchronous pipes to serialize reads and writes to the
	// underlying websocket.Conn.
	//
	// https://godoc.org/github.com/gorilla/websocket#hdr-Concurrency
	// "Connections support one concurrent reader and one concurrent writer.
	// Applications are responsible for ensuring that no more than one
	// goroutine calls the write methods (NextWriter, etc.) concurrently and
	// that no more than one goroutine calls the read methods (NextReader,
	// etc.) concurrently. The Close and WriteControl methods can be called
	// concurrently with all other methods."
	pr1, pw1 := io.Pipe()
	go func() {
		pw1.CloseWithError(closeErrorToEOF(readLoop(pw1, ws)))
	}()
	pr2, pw2 := io.Pipe()
	go func() {
		pr2.CloseWithError(closeErrorToEOF(writeLoop(ws, pr2)))
	}()
	return &Conn{
		ws:     ws,
		Reader: pr1,
		Writer: pw2,
	}
}
