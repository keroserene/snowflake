package lib

import (
	"log"
	"time"
)

const (
	LogTimeInterval = 5
)

type BytesLogger interface {
	AddOutbound(int)
	AddInbound(int)
}

// Default BytesLogger does nothing.
type BytesNullLogger struct{}

func (b BytesNullLogger) AddOutbound(amount int) {}
func (b BytesNullLogger) AddInbound(amount int)  {}

// BytesSyncLogger uses channels to safely log from multiple sources with output
// occuring at reasonable intervals.
type BytesSyncLogger struct {
	outboundChan chan int
	inboundChan  chan int
}

// NewBytesSyncLogger returns a new BytesSyncLogger and starts it loggin.
func NewBytesSyncLogger() *BytesSyncLogger {
	b := &BytesSyncLogger{
		outboundChan: make(chan int, 5),
		inboundChan:  make(chan int, 5),
	}
	go b.log()
	return b
}

func (b *BytesSyncLogger) log() {
	var outbound, inbound, outEvents, inEvents int
	output := func() {
		log.Printf("Traffic Bytes (in|out): %d | %d -- (%d OnMessages, %d Sends)",
			inbound, outbound, inEvents, outEvents)
		outbound = 0
		outEvents = 0
		inbound = 0
		inEvents = 0
	}
	last := time.Now()
	for {
		select {
		case amount := <-b.outboundChan:
			outbound += amount
			outEvents++
			if time.Since(last) > time.Second*LogTimeInterval {
				last = time.Now()
				output()
			}
		case amount := <-b.inboundChan:
			inbound += amount
			inEvents++
			if time.Since(last) > time.Second*LogTimeInterval {
				last = time.Now()
				output()
			}
		case <-time.After(time.Second * LogTimeInterval):
			if inEvents > 0 || outEvents > 0 {
				output()
			}
		}
	}
}

func (b *BytesSyncLogger) AddOutbound(amount int) {
	b.outboundChan <- amount
}

func (b *BytesSyncLogger) AddInbound(amount int) {
	b.inboundChan <- amount
}
