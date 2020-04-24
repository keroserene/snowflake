package lib

import (
	"io"
	"net"
)

// Interface for catching Snowflakes. (aka the remote dialer)
type Tongue interface {
	Catch() (*WebRTCPeer, error)
}

// Interface for collecting some number of Snowflakes, for passing along
// ultimately to the SOCKS handler.
type SnowflakeCollector interface {
	// Add a Snowflake to the collection.
	// Implementation should decide how to connect and maintain the webRTCConn.
	Collect() (*WebRTCPeer, error)

	// Remove and return the most available Snowflake from the collection.
	Pop() *WebRTCPeer

	// Signal when the collector has stopped collecting.
	Melted() <-chan struct{}
}

// Interface to adapt to goptlib's SocksConn struct.
type SocksConnector interface {
	Grant(*net.TCPAddr) error
	Reject() error
	net.Conn
}

// Interface for the Snowflake's transport. (Typically just webrtc.DataChannel)
type SnowflakeDataChannel interface {
	io.Closer
	Send([]byte) error
}
