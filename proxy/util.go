package main

import (
	"fmt"
	"time"
)

type BytesLogger interface {
	AddOutbound(int)
	AddInbound(int)
	ThroughputSummary() string
}

// Default BytesLogger does nothing.
type BytesNullLogger struct{}

func (b BytesNullLogger) AddOutbound(amount int)    {}
func (b BytesNullLogger) AddInbound(amount int)     {}
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

func (b *BytesSyncLogger) AddOutbound(amount int) {
	b.outboundChan <- amount
}

func (b *BytesSyncLogger) AddInbound(amount int) {
	b.inboundChan <- amount
}

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
