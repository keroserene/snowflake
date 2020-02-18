package websocketconn

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// Returns a (server, client) pair of websocketconn.Conns.
func connPair() (*Conn, *Conn, error) {
	// Will be assigned inside server.Handler.
	var serverConn *Conn

	// Start up a web server to receive the request.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}
	defer ln.Close()
	errCh := make(chan error)
	server := http.Server{
		Handler: http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			upgrader := websocket.Upgrader{
				CheckOrigin: func(*http.Request) bool { return true },
			}
			ws, err := upgrader.Upgrade(rw, req, nil)
			if err != nil {
				errCh <- err
				return
			}
			serverConn = New(ws)
			close(errCh)
		}),
	}
	defer server.Close()
	go func() {
		err := server.Serve(ln)
		if err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Make a request to the web server.
	urlStr := (&url.URL{Scheme: "ws", Host: ln.Addr().String()}).String()
	ws, _, err := (&websocket.Dialer{}).Dial(urlStr, nil)
	if err != nil {
		return nil, nil, err
	}
	clientConn := New(ws)

	// The server is finished when errCh is written to or closed.
	err = <-errCh
	if err != nil {
		return nil, nil, err
	}
	return serverConn, clientConn, nil
}

// Test that you can write in chunks and read the result concatenated.
func TestWrite(t *testing.T) {
	tests := [][][]byte{
		{},
		{[]byte("foo")},
		{[]byte("foo"), []byte("bar")},
		{{}, []byte("foo"), {}, {}, []byte("bar")},
	}

	for _, test := range tests {
		s, c, err := connPair()
		if err != nil {
			t.Fatal(err)
		}

		// This is a little awkward because we need to read to and write
		// from both ends of the Conn, and we need to do it in separate
		// goroutines because otherwise a Write may block waiting for
		// someone to Read it. Here we set up a loop in a separate
		// goroutine, reading from the Conn s and writing to the dataCh
		// and errCh channels, whose ultimate effect in the select loop
		// below is like
		//   data, err := ioutil.ReadAll(s)
		dataCh := make(chan []byte)
		errCh := make(chan error)
		go func() {
			for {
				var buf [1024]byte
				n, err := s.Read(buf[:])
				if err != nil {
					errCh <- err
					return
				}
				p := make([]byte, n)
				copy(p, buf[:])
				dataCh <- p
			}
		}()

		// Write the data to the client side of the Conn, one chunk at a
		// time.
		for i, chunk := range test {
			n, err := c.Write(chunk)
			if err != nil || n != len(chunk) {
				t.Fatalf("%+q Write chunk %d: got (%d, %v), expected (%d, %v)",
					test, i, n, err, len(chunk), nil)
			}
		}
		// We cannot immediately c.Close here, because that closes the
		// connection right away, without waiting for buffered data to
		// be sent.

		// Pull data and err from the server goroutine above.
		var data []byte
		err = nil
	loop:
		for {
			select {
			case p := <-dataCh:
				data = append(data, p...)
			case err = <-errCh:
				break loop
			case <-time.After(100 * time.Millisecond):
				break loop
			}
		}
		s.Close()
		c.Close()

		// Now data and err contain the result of reading everything
		// from s.
		expected := bytes.Join(test, []byte{})
		if err != nil || !bytes.Equal(data, expected) {
			t.Fatalf("%+q ReadAll: got (%+q, %v), expected (%+q, %v)",
				test, data, err, expected, nil)
		}
	}
}

// Test that multiple goroutines may call Read on a Conn simultaneously. Run
// this with
//   go test -race
func TestConcurrentRead(t *testing.T) {
	s, c, err := connPair()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// Set up multiple threads reading from the same conn.
	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			_, err := io.Copy(ioutil.Discard, s)
			if err != nil {
				errCh <- err
			}
		}()
	}

	// Write a bunch of data to the other end.
	for i := 0; i < 2000; i++ {
		_, err := fmt.Fprintf(c, "%d", i)
		if err != nil {
			c.Close()
			t.Fatalf("Write: %v", err)
		}
	}
	c.Close()

	wg.Wait()
	close(errCh)

	err = <-errCh
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
}

// Test that multiple goroutines may call Write on a Conn simultaneously. Run
// this with
//   go test -race
func TestConcurrentWrite(t *testing.T) {
	s, c, err := connPair()
	if err != nil {
		t.Fatal(err)
	}

	// Set up multiple threads writing to the same conn.
	errCh := make(chan error, 3)
	var wg sync.WaitGroup
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				_, err := fmt.Fprintf(s, "%d", j)
				if err != nil {
					errCh <- err
					break
				}
			}
		}()
	}
	go func() {
		wg.Wait()
		err := s.Close()
		if err != nil {
			errCh <- err
		}
		close(errCh)
	}()

	// Read from the other end.
	_, err = io.Copy(ioutil.Discard, c)
	c.Close()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	err = <-errCh
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
}

// Test that Read and Write methods return errors after Close.
func TestClose(t *testing.T) {
	s, c, err := connPair()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	err = s.Close()
	if err != nil {
		t.Fatal(err)
	}

	var buf [10]byte
	n, err := s.Read(buf[:])
	if n != 0 || err == nil {
		t.Fatalf("Read after Close returned (%v, %v), expected (%v, non-nil)", n, err, 0)
	}

	_, err = s.Write([]byte{1, 2, 3})
	// Here we break the abstraction a little and look for a specific error,
	// io.ErrClosedPipe. This is because we know the Conn uses an io.Pipe
	// internally.
	if err != io.ErrClosedPipe {
		t.Fatalf("Write after Close returned %v, expected %v", err, io.ErrClosedPipe)
	}
}
