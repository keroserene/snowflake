package lib

import (
	"bufio"
	"errors"
	"io"
	"net"
	"time"

	"git.torproject.org/pluggable-transports/snowflake.git/common/encapsulation"
)

var errNotImplemented = errors.New("not implemented")

// EncapsulationPacketConn implements the net.PacketConn interface over an
// io.ReadWriteCloser stream, using the encapsulation package to represent
// packets in a stream.
type EncapsulationPacketConn struct {
	io.ReadWriteCloser
	localAddr  net.Addr
	remoteAddr net.Addr
	bw         *bufio.Writer
}

// NewEncapsulationPacketConn makes
func NewEncapsulationPacketConn(
	localAddr, remoteAddr net.Addr,
	conn io.ReadWriteCloser,
) *EncapsulationPacketConn {
	return &EncapsulationPacketConn{
		ReadWriteCloser: conn,
		localAddr:       localAddr,
		remoteAddr:      remoteAddr,
		bw:              bufio.NewWriter(conn),
	}
}

// ReadFrom reads an encapsulated packet from the stream.
func (c *EncapsulationPacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	data, err := encapsulation.ReadData(c.ReadWriteCloser)
	if err != nil {
		return 0, c.remoteAddr, err
	}
	return copy(p, data), c.remoteAddr, nil
}

// WriteTo writes an encapsulated packet to the stream.
func (c *EncapsulationPacketConn) WriteTo(p []byte, addr net.Addr) (int, error) {
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
func (c *EncapsulationPacketConn) LocalAddr() net.Addr {
	return c.localAddr
}

func (c *EncapsulationPacketConn) SetDeadline(t time.Time) error      { return errNotImplemented }
func (c *EncapsulationPacketConn) SetReadDeadline(t time.Time) error  { return errNotImplemented }
func (c *EncapsulationPacketConn) SetWriteDeadline(t time.Time) error { return errNotImplemented }
