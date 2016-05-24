package main

import (
	"net"
)

// Interface for collecting and releasing snowflakes.
type SnowflakeCollector interface {
	Collect() (*webRTCConn, error)
	Release() *webRTCConn
}

// Interface for catching those wild Snowflakes.
type Tongue interface {
	Catch() (*webRTCConn, error)
}

// Interface which primarily adapts to goptlib's SocksConn struct.
type SocksConnector interface {
	Grant(*net.TCPAddr) error
	Reject() error
	net.Conn
}

// Interface for the Snowflake's transport.
// (Specifically, webrtc.DataChannel)
type SnowflakeChannel interface {
	Send([]byte)
	Close() error
}
