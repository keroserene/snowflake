package lib

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"time"

	"git.torproject.org/pluggable-transports/snowflake.git/common/turbotunnel"
	"github.com/xtaci/kcp-go/v5"
	"github.com/xtaci/smux"
)

const (
	ReconnectTimeout = 10 * time.Second
	SnowflakeTimeout = 30 * time.Second
)

type dummyAddr struct{}

func (addr dummyAddr) Network() string { return "dummy" }
func (addr dummyAddr) String() string  { return "dummy" }

// Given an accepted SOCKS connection, establish a WebRTC connection to the
// remote peer and exchange traffic.
func Handler(socks net.Conn, snowflakes SnowflakeCollector) error {
	clientID := turbotunnel.NewClientID()

	// We build a persistent KCP session on a sequence of ephemeral WebRTC
	// connections. This dialContext tells RedialPacketConn how to get a new
	// WebRTC connection when the previous one dies. Inside each WebRTC
	// connection, we use EncapsulationPacketConn to encode packets into a
	// stream.
	dialContext := func(ctx context.Context) (net.PacketConn, error) {
		log.Printf("redialing on same connection")
		// Obtain an available WebRTC remote. May block.
		conn := snowflakes.Pop()
		if conn == nil {
			return nil, errors.New("handler: Received invalid Snowflake")
		}
		log.Println("---- Handler: snowflake assigned ----")
		// Send the magic Turbo Tunnel token.
		_, err := conn.Write(turbotunnel.Token[:])
		if err != nil {
			return nil, err
		}
		// Send ClientID prefix.
		_, err = conn.Write(clientID[:])
		if err != nil {
			return nil, err
		}
		return NewEncapsulationPacketConn(dummyAddr{}, dummyAddr{}, conn), nil
	}
	pconn := turbotunnel.NewRedialPacketConn(dummyAddr{}, dummyAddr{}, dialContext)
	defer pconn.Close()

	// conn is built on the underlying RedialPacketConn—when one WebRTC
	// connection dies, another one will be found to take its place. The
	// sequence of packets across multiple WebRTC connections drives the KCP
	// engine.
	conn, err := kcp.NewConn2(dummyAddr{}, nil, 0, 0, pconn)
	if err != nil {
		return err
	}
	defer conn.Close()
	// Permit coalescing the payloads of consecutive sends.
	conn.SetStreamMode(true)
	// Disable the dynamic congestion window (limit only by the
	// maximum of local and remote static windows).
	conn.SetNoDelay(
		0, // default nodelay
		0, // default interval
		0, // default resend
		1, // nc=1 => congestion window off
	)
	// On the KCP connection we overlay an smux session and stream.
	smuxConfig := smux.DefaultConfig()
	smuxConfig.Version = 2
	smuxConfig.KeepAliveTimeout = 10 * time.Minute
	sess, err := smux.Client(conn, smuxConfig)
	if err != nil {
		return err
	}
	defer sess.Close()
	stream, err := sess.OpenStream()
	if err != nil {
		return err
	}
	defer stream.Close()

	// Begin exchanging data.
	log.Printf("---- Handler: begin stream %v ---", stream.ID())
	copyLoop(socks, stream)
	log.Printf("---- Handler: closed stream %v ---", stream.ID())
	return nil
}

// Exchanges bytes between two ReadWriters.
// (In this case, between a SOCKS connection and smux stream.)
func copyLoop(socks, stream io.ReadWriter) {
	done := make(chan struct{}, 2)
	go func() {
		if _, err := io.Copy(socks, stream); err != nil {
			log.Printf("copying WebRTC to SOCKS resulted in error: %v", err)
		}
		done <- struct{}{}
	}()
	go func() {
		if _, err := io.Copy(stream, socks); err != nil {
			log.Printf("copying SOCKS to stream resulted in error: %v", err)
		}
		done <- struct{}{}
	}()
	<-done
	log.Println("copy loop ended")
}
