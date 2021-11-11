package snowflake_client

import (
	"bufio"
	"errors"
	"io"
	"net"
	"time"

	"git.torproject.org/pluggable-transports/snowflake.git/v2/common/encapsulation"
)

var errNotImplemented = errors.New("not implemented")

// encapsulationPacketConn implements the net.PacketConn interface over an
// io.ReadWriteCloser stream, using the encapsulation package to represent
// packets in a stream.
type encapsulationPacketConn struct {
	io.ReadWriteCloser
	localAddr  net.Addr
	remoteAddr net.Addr
	bw         *bufio.Writer
}

// NewEncapsulationPacketConn makes
func newEncapsulationPacketConn(
	localAddr, remoteAddr net.Addr,
	conn io.ReadWriteCloser,
) *encapsulationPacketConn {
	return &encapsulationPacketConn{
		ReadWriteCloser: conn,
		localAddr:       localAddr,
		remoteAddr:      remoteAddr,
		bw:              bufio.NewWriter(conn),
	}
}

// ReadFrom reads an encapsulated packet from the stream.
func (c *encapsulationPacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	data, err := encapsulation.ReadData(c.ReadWriteCloser)
	if err != nil {
		return 0, c.remoteAddr, err
	}
	return copy(p, data), c.remoteAddr, nil
}

// WriteTo writes an encapsulated packet to the stream.
func (c *encapsulationPacketConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	// addr is ignored.
	_, err := encapsulation.WriteData(c.bw, p)
	if err == nil {
		err = c.bw.Flush()
	}
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

// LocalAddr returns the localAddr value that was passed to
// NewEncapsulationPacketConn.
func (c *encapsulationPacketConn) LocalAddr() net.Addr {
	return c.localAddr
}

func (c *encapsulationPacketConn) SetDeadline(t time.Time) error      { return errNotImplemented }
func (c *encapsulationPacketConn) SetReadDeadline(t time.Time) error  { return errNotImplemented }
func (c *encapsulationPacketConn) SetWriteDeadline(t time.Time) error { return errNotImplemented }
