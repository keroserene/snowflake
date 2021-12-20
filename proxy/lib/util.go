package snowflake_proxy

import (
	"fmt"
	"time"
)

// bytesLogger is an interface which is used to allow logging the throughput
// of the Snowflake. A default bytesLogger(bytesNullLogger) does nothing.
type bytesLogger interface {
	AddOutbound(int)
	AddInbound(int)
	ThroughputSummary() string
	GetStat() (in int, out int)
}

// bytesNullLogger Default bytesLogger does nothing.
type bytesNullLogger struct{}

// AddOutbound in bytesNullLogger does nothing
func (b bytesNullLogger) AddOutbound(amount int) {}

// AddInbound in bytesNullLogger does nothing
func (b bytesNullLogger) AddInbound(amount int) {}

// ThroughputSummary in bytesNullLogger does nothing
func (b bytesNullLogger) ThroughputSummary() string { return "" }

func (b bytesNullLogger) GetStat() (in int, out int) { return -1, -1 }

// bytesSyncLogger uses channels to safely log from multiple sources with output
// occuring at reasonable intervals.
type bytesSyncLogger struct {
	outboundChan, inboundChan              chan int
	outbound, inbound, outEvents, inEvents int
	start                                  time.Time
}

// newBytesSyncLogger returns a new bytesSyncLogger and starts it loggin.
func newBytesSyncLogger() *bytesSyncLogger {
	b := &bytesSyncLogger{
		outboundChan: make(chan int, 5),
		inboundChan:  make(chan int, 5),
	}
	go b.log()
	b.start = time.Now()
	return b
}

func (b *bytesSyncLogger) log() {
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
func (b *bytesSyncLogger) AddOutbound(amount int) {
	b.outboundChan <- amount
}

// AddInbound add a number of bytes to the inbound total reported by the logger
func (b *bytesSyncLogger) AddInbound(amount int) {
	b.inboundChan <- amount
}

// ThroughputSummary view a formatted summary of the throughput totals
func (b *bytesSyncLogger) ThroughputSummary() string {
	inbound := b.inbound
	outbound := b.outbound

	inbound, inUnit := formatTraffic(inbound)
	outbound, outUnit := formatTraffic(outbound)

	t := time.Now()
	return fmt.Sprintf("Traffic throughput (up|down): %d %s|%d %s -- (%d OnMessages, %d Sends, over %d seconds)", inbound, inUnit, outbound, outUnit, b.outEvents, b.inEvents, int(t.Sub(b.start).Seconds()))
}

func (b *bytesSyncLogger) GetStat() (in int, out int) { return b.inbound, b.outbound }

func formatTraffic(amount int) (value int, unit string) {
	value = amount
	units := []string{"B", "KB", "MB", "GB"}
	for i, u := range units {
		unit = u
		if (value < 1000) || (i == len(units)-1) {
			break
		}
		value = value / 1000
	}
	return

}
