package main

import (
	"net"
	"net/http"
	"strconv"
	"testing"

	"git.torproject.org/pluggable-transports/snowflake.git/common/websocketconn"
	"github.com/gorilla/websocket"
	. "github.com/smartystreets/goconvey/convey"
)

func TestClientAddr(t *testing.T) {
	Convey("Testing clientAddr", t, func() {
		// good tests
		for _, test := range []struct {
			input    string
			expected net.IP
		}{
			{"1.2.3.4", net.ParseIP("1.2.3.4")},
			{"1:2::3:4", net.ParseIP("1:2::3:4")},
		} {
			useraddr := clientAddr(test.input)
			host, port, err := net.SplitHostPort(useraddr)
			if err != nil {
				t.Errorf("clientAddr(%q) → SplitHostPort error %v", test.input, err)
				continue
			}
			if !test.expected.Equal(net.ParseIP(host)) {
				t.Errorf("clientAddr(%q) → host %q, not %v", test.input, host, test.expected)
			}
			portNo, err := strconv.Atoi(port)
			if err != nil {
				t.Errorf("clientAddr(%q) → port %q", test.input, port)
				continue
			}
			if portNo == 0 {
				t.Errorf("clientAddr(%q) → port %d", test.input, portNo)
			}
		}

		// bad tests
		for _, input := range []string{
			"",
			"abc",
			"1.2.3.4.5",
			"[12::34]",
		} {
			useraddr := clientAddr(input)
			if useraddr != "" {
				t.Errorf("clientAddr(%q) → %q, not %q", input, useraddr, "")
			}
		}
	})
}

type StubHandler struct{}

func (handler *StubHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ws, _ := upgrader.Upgrade(w, r, nil)

	conn := websocketconn.NewWebSocketConn(ws)
	defer conn.Close()

	//dial stub OR
	or, _ := net.DialTCP("tcp", nil, &net.TCPAddr{IP: net.ParseIP("localhost"), Port: 8889})

	proxy(or, &conn)
}

func Test(t *testing.T) {
	Convey("Websocket server", t, func() {
		//Set up the snowflake web server
		ipStr, portStr, _ := net.SplitHostPort(":8888")
		port, _ := strconv.ParseUint(portStr, 10, 16)
		addr := &net.TCPAddr{IP: net.ParseIP(ipStr), Port: int(port)}
		Convey("We don't listen on port 0", func() {
			addr = &net.TCPAddr{IP: net.ParseIP(ipStr), Port: 0}
			server, err := initServer(addr, nil,
				func(server *http.Server, errChan chan<- error) {
					return
				})
			So(err, ShouldNotBeNil)
			So(server, ShouldBeNil)
		})

		Convey("Plain HTTP server accepts connections", func(c C) {
			server, err := startServer(addr)
			So(err, ShouldBeNil)

			ws, _, err := websocket.DefaultDialer.Dial("ws://localhost:8888", nil)
			wsConn := websocketconn.NewWebSocketConn(ws)
			So(err, ShouldEqual, nil)
			So(wsConn, ShouldNotEqual, nil)

			server.Close()
			wsConn.Close()

		})
		Convey("Handler proxies data", func(c C) {

			laddr := &net.TCPAddr{IP: net.ParseIP("localhost"), Port: 8889}

			go func() {

				//stub OR
				listener, err := net.ListenTCP("tcp", laddr)
				c.So(err, ShouldBeNil)
				conn, err := listener.Accept()
				c.So(err, ShouldBeNil)

				b := make([]byte, 5)
				n, err := conn.Read(b)
				c.So(err, ShouldBeNil)
				c.So(n, ShouldEqual, 5)
				c.So(b, ShouldResemble, []byte("Hello"))

				n, err = conn.Write([]byte("world!"))
				c.So(n, ShouldEqual, 6)
				c.So(err, ShouldBeNil)
			}()

			//overwite handler
			server, err := initServer(addr, nil,
				func(server *http.Server, errChan chan<- error) {
					server.ListenAndServe()
				})
			So(err, ShouldBeNil)

			var handler StubHandler
			server.Handler = &handler

			ws, _, err := websocket.DefaultDialer.Dial("ws://localhost:8888", nil)
			So(err, ShouldEqual, nil)
			wsConn := websocketconn.NewWebSocketConn(ws)
			So(wsConn, ShouldNotEqual, nil)

			wsConn.Write([]byte("Hello"))
			b := make([]byte, 6)
			n, err := wsConn.Read(b)
			So(n, ShouldEqual, 6)
			So(b, ShouldResemble, []byte("world!"))

			wsConn.Close()
			server.Close()

		})

	})
}
