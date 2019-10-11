package lib

import (
	"errors"
	"io"
	"log"
	"net"
	"sync"
)

const (
	ReconnectTimeout = 10
	SnowflakeTimeout = 30
)

// Given an accepted SOCKS connection, establish a WebRTC connection to the
// remote peer and exchange traffic.
func Handler(socks SocksConnector, snowflakes SnowflakeCollector) error {
	// Obtain an available WebRTC remote. May block.
	snowflake := snowflakes.Pop()
	if nil == snowflake {
		if err := socks.Reject(); err != nil {
			log.Printf("socks.Reject returned error: %v", err)
		}

		return errors.New("handler: Received invalid Snowflake")
	}
	defer socks.Close()
	defer snowflake.Close()
	log.Println("---- Handler: snowflake assigned ----")
	err := socks.Grant(&net.TCPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return err
	}

	go func() {
		// When WebRTC resets, close the SOCKS connection too.
		snowflake.WaitForReset()
		socks.Close()
	}()

	// Begin exchanging data. Either WebRTC or localhost SOCKS will close first.
	// In eithercase, this closes the handler and induces a new handler.
	copyLoop(socks, snowflake)
	log.Println("---- Handler: closed ---")
	return nil
}

// Exchanges bytes between two ReadWriters.
// (In this case, between a SOCKS and WebRTC connection.)
func copyLoop(WebRTC, SOCKS io.ReadWriter) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		if _, err := io.Copy(SOCKS, WebRTC); err != nil {
			log.Printf("copying WebRTC to SOCKS resulted in error: %v", err)
		}
		wg.Done()
	}()
	go func() {
		if _, err := io.Copy(WebRTC, SOCKS); err != nil {
			log.Printf("copying SOCKS to WebRTC resulted in error: %v", err)
		}
		wg.Done()
	}()
	wg.Wait()
	log.Println("copy loop ended")
}
