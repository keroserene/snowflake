package lib

import (
	"errors"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

const (
	ReconnectTimeout = 10 * time.Second
	SnowflakeTimeout = 30 * time.Second
)

// Given an accepted SOCKS connection, establish a WebRTC connection to the
// remote peer and exchange traffic.
func Handler(socks net.Conn, snowflakes SnowflakeCollector) error {
	// Obtain an available WebRTC remote. May block.
	snowflake := snowflakes.Pop()
	if nil == snowflake {
		return errors.New("handler: Received invalid Snowflake")
	}
	defer snowflake.Close()
	log.Println("---- Handler: snowflake assigned ----")

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
func copyLoop(socks, webRTC io.ReadWriter) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		if _, err := io.Copy(socks, webRTC); err != nil {
			log.Printf("copying WebRTC to SOCKS resulted in error: %v", err)
		}
		wg.Done()
	}()
	go func() {
		if _, err := io.Copy(webRTC, socks); err != nil {
			log.Printf("copying SOCKS to WebRTC resulted in error: %v", err)
		}
		wg.Done()
	}()
	wg.Wait()
	log.Println("copy loop ended")
}
