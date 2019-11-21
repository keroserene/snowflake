package websocketconn

import (
	"net"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWebsocketConn(t *testing.T) {
	Convey("CopyLoop", t, func() {
		c1, s1 := net.Pipe()
		c2, s2 := net.Pipe()
		go CopyLoop(s1, s2)
		go func() {
			bytes := []byte("Hello!")
			c1.Write(bytes)
		}()
		bytes := make([]byte, 6)
		n, err := c2.Read(bytes)
		So(n, ShouldEqual, 6)
		So(err, ShouldEqual, nil)
		So(bytes, ShouldResemble, []byte("Hello!"))
		s1.Close()

		// Check that copy loop has closed other connection
		_, err = s2.Write(bytes)
		So(err, ShouldNotBeNil)
	})
}
