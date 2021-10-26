package snowflake

import (
	"fmt"
	"time"
)

// BytesLogger is an interface which is used to allow logging the throughput
// of the Snowflake. A default BytesLogger(BytesNullLogger) does nothing.
type BytesLogger interface {
	AddOutbound(int)
	AddInbound(int)
	ThroughputSummary() string
}

// BytesNullLogger Default BytesLogger does nothing.
type BytesNullLogger struct{}

// AddOutbound in BytesNullLogger does nothing
func (b BytesNullLogger) AddOutbound(amount int) {}

// AddInbound in BytesNullLogger does nothing
func (b BytesNullLogger) AddInbound(amount int) {}

// ThroughputSummary in BytesNullLogger does nothing
func (b BytesNullLogger) ThroughputSummary() string { return "" }

// BytesSyncLogger uses channels to safely log from multiple sources with output
// occuring at reasonable intervals.
type BytesSyncLogger struct {
	outboundChan, inboundChan              chan int
	outbound, inbound, outEvents, inEvents int
	start                                  time.Time
}

// NewBytesSyncLogger returns a new BytesSyncLogger and starts it loggin.
func NewBytesSyncLogger() *BytesSyncLogger {
	b := &BytesSyncLogger{
		outboundChan: make(chan int, 5),
		inboundChan:  make(chan int, 5),
	}
	go b.log()
	b.start = time.Now()
	return b
}

func (b *BytesSyncLogger) log() {
	for {
		select {
		case amount := <-b.outboundChan:
			b.outbound += amount
			b.outEvents++
		case amount := <-b.inboundChan:
			b.inbound += amount
			b.inEvents++
		}
	}
}

// AddOutbound add a number of bytes to the outbound total reported by the logger
func (b *BytesSyncLogger) AddOutbound(amount int) {
	b.outboundChan <- amount
}

// AddInbound add a number of bytes to the inbound total reported by the logger
func (b *BytesSyncLogger) AddInbound(amount int) {
	b.inboundChan <- amount
}

// ThroughputSummary view a formatted summary of the throughput totals
func (b *BytesSyncLogger) ThroughputSummary() string {
	var inUnit, outUnit string
	units := []string{"B", "KB", "MB", "GB"}

	inbound := b.inbound
	outbound := b.outbound

	for i, u := range units {
		inUnit = u
		if (inbound < 1000) || (i == len(units)-1) {
			break
		}
		inbound = inbound / 1000
	}
	for i, u := range units {
		outUnit = u
		if (outbound < 1000) || (i == len(units)-1) {
			break
		}
		outbound = outbound / 1000
	}
	t := time.Now()
	return fmt.Sprintf("Traffic throughput (up|down): %d %s|%d %s -- (%d OnMessages, %d Sends, over %d seconds)", inbound, inUnit, outbound, outUnit, b.outEvents, b.inEvents, int(t.Sub(b.start).Seconds()))
}
