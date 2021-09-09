package snowflake_client

import (
	"log"
	"time"
)

const (
	LogTimeInterval = 5 * time.Second
)

type bytesLogger interface {
	addOutbound(int)
	addInbound(int)
}

// Default bytesLogger does nothing.
type bytesNullLogger struct{}

func (b bytesNullLogger) addOutbound(amount int) {}
func (b bytesNullLogger) addInbound(amount int)  {}

// bytesSyncLogger uses channels to safely log from multiple sources with output
// occuring at reasonable intervals.
type bytesSyncLogger struct {
	outboundChan chan int
	inboundChan  chan int
}

// newBytesSyncLogger returns a new bytesSyncLogger and starts it loggin.
func newBytesSyncLogger() *bytesSyncLogger {
	b := &bytesSyncLogger{
		outboundChan: make(chan int, 5),
		inboundChan:  make(chan int, 5),
	}
	go b.log()
	return b
}

func (b *bytesSyncLogger) log() {
	var outbound, inbound, outEvents, inEvents int
	ticker := time.NewTicker(LogTimeInterval)
	for {
		select {
		case <-ticker.C:
			if outEvents > 0 || inEvents > 0 {
				log.Printf("Traffic Bytes (in|out): %d | %d -- (%d OnMessages, %d Sends)",
					inbound, outbound, inEvents, outEvents)
			}
			outbound = 0
			outEvents = 0
			inbound = 0
			inEvents = 0
		case amount := <-b.outboundChan:
			outbound += amount
			outEvents++
		case amount := <-b.inboundChan:
			inbound += amount
			inEvents++
		}
	}
}

func (b *bytesSyncLogger) addOutbound(amount int) {
	b.outboundChan <- amount
}

func (b *bytesSyncLogger) addInbound(amount int) {
	b.inboundChan <- amount
}
