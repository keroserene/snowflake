package turbotunnel

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// RedialPacketConn implements a long-lived net.PacketConn atop a sequence of
// other, transient net.PacketConns. RedialPacketConn creates a new
// net.PacketConn by calling a provided dialContext function. Whenever the
// net.PacketConn experiences a ReadFrom or WriteTo error, RedialPacketConn
// calls the dialContext function again and starts sending and receiving packets
// on the new net.PacketConn. RedialPacketConn's own ReadFrom and WriteTo
// methods return an error only when the dialContext function returns an error.
//
// RedialPacketConn uses static local and remote addresses that are independent
// of those of any dialed net.PacketConn.
type RedialPacketConn struct {
	localAddr   net.Addr
	remoteAddr  net.Addr
	dialContext func(context.Context) (net.PacketConn, error)
	recvQueue   chan []byte
	sendQueue   chan []byte
	closed      chan struct{}
	closeOnce   sync.Once
	// The first dial error, which causes the clientPacketConn to be
	// closed and is returned from future read/write operations. Compare to
	// the rerr and werr in io.Pipe.
	err atomic.Value
}

// NewQueuePacketConn makes a new RedialPacketConn, with the given static local
// and remote addresses, and dialContext function.
func NewRedialPacketConn(
	localAddr, remoteAddr net.Addr,
	dialContext func(context.Context) (net.PacketConn, error),
) *RedialPacketConn {
	c := &RedialPacketConn{
		localAddr:   localAddr,
		remoteAddr:  remoteAddr,
		dialContext: dialContext,
		recvQueue:   make(chan []byte, queueSize),
		sendQueue:   make(chan []byte, queueSize),
		closed:      make(chan struct{}),
		err:         atomic.Value{},
	}
	go c.dialLoop()
	return c
}

// dialLoop repeatedly calls c.dialContext and passes the resulting
// net.PacketConn to c.exchange. It returns only when c is closed or dialContext
// returns an error.
func (c *RedialPacketConn) dialLoop() {
	ctx, cancel := context.WithCancel(context.Background())
	for {
		select {
		case <-c.closed:
			cancel()
			return
		default:
		}
		conn, err := c.dialContext(ctx)
		if err != nil {
			c.closeWithError(err)
			cancel()
			return
		}
		c.exchange(conn)
		conn.Close()
	}
}

// exchange calls ReadFrom on the given net.PacketConn and places the resulting
// packets in the receive queue, and takes packets from the send queue and calls
// WriteTo on them, making the current net.PacketConn active.
func (c *RedialPacketConn) exchange(conn net.PacketConn) {
	readErrCh := make(chan error)
	writeErrCh := make(chan error)

	go func() {
		defer close(readErrCh)
		for {
			select {
			case <-c.closed:
				return
			case <-writeErrCh:
				return
			default:
			}

			var buf [1500]byte
			n, _, err := conn.ReadFrom(buf[:])
			if err != nil {
				readErrCh <- err
				return
			}
			p := make([]byte, n)
			copy(p, buf[:])
			select {
			case c.recvQueue <- p:
			default: // OK to drop packets.
			}
		}
	}()

	go func() {
		defer close(writeErrCh)
		for {
			select {
			case <-c.closed:
				return
			case <-readErrCh:
				return
			case p := <-c.sendQueue:
				_, err := conn.WriteTo(p, c.remoteAddr)
				if err != nil {
					writeErrCh <- err
					return
				}
			}
		}
	}()

	select {
	case <-readErrCh:
	case <-writeErrCh:
	}
}

// ReadFrom reads a packet from the currently active net.PacketConn. The
// packet's original remote address is replaced with the RedialPacketConn's own
// remote address.
func (c *RedialPacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	select {
	case <-c.closed:
		return 0, nil, &net.OpError{Op: "read", Net: c.LocalAddr().Network(), Source: c.LocalAddr(), Addr: c.remoteAddr, Err: c.err.Load().(error)}
	default:
	}
	select {
	case <-c.closed:
		return 0, nil, &net.OpError{Op: "read", Net: c.LocalAddr().Network(), Source: c.LocalAddr(), Addr: c.remoteAddr, Err: c.err.Load().(error)}
	case buf := <-c.recvQueue:
		return copy(p, buf), c.remoteAddr, nil
	}
}

// WriteTo writes a packet to the currently active net.PacketConn. The addr
// argument is ignored and instead replaced with the RedialPacketConn's own
// remote address.
func (c *RedialPacketConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	// addr is ignored.
	select {
	case <-c.closed:
		return 0, &net.OpError{Op: "write", Net: c.LocalAddr().Network(), Source: c.LocalAddr(), Addr: c.remoteAddr, Err: c.err.Load().(error)}
	default:
	}
	buf := make([]byte, len(p))
	copy(buf, p)
	select {
	case c.sendQueue <- buf:
		return len(buf), nil
	default:
		// Drop the outgoing packet if the send queue is full.
		return len(buf), nil
	}
}

// closeWithError unblocks pending operations and makes future operations fail
// with the given error. If err is nil, it becomes errClosedPacketConn.
func (c *RedialPacketConn) closeWithError(err error) error {
	var once bool
	c.closeOnce.Do(func() {
		// Store the error to be returned by future read/write
		// operations.
		if err == nil {
			err = errors.New("operation on closed connection")
		}
		c.err.Store(err)
		close(c.closed)
		once = true
	})
	if !once {
		return &net.OpError{Op: "close", Net: c.LocalAddr().Network(), Addr: c.LocalAddr(), Err: c.err.Load().(error)}
	}
	return nil
}

// Close unblocks pending operations and makes future operations fail with a
// "closed connection" error.
func (c *RedialPacketConn) Close() error {
	return c.closeWithError(nil)
}

// LocalAddr returns the localAddr value that was passed to NewRedialPacketConn.
func (c *RedialPacketConn) LocalAddr() net.Addr { return c.localAddr }

func (c *RedialPacketConn) SetDeadline(t time.Time) error      { return errNotImplemented }
func (c *RedialPacketConn) SetReadDeadline(t time.Time) error  { return errNotImplemented }
func (c *RedialPacketConn) SetWriteDeadline(t time.Time) error { return errNotImplemented }
