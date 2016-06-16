package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/keroserene/go-webrtc"
)

const (
	LogTimeInterval = 5
)

type IceServerList []webrtc.ConfigurationOption

func (i *IceServerList) String() string {
	return fmt.Sprint(*i)
}

func (i *IceServerList) Set(s string) error {
	log.Println("IceServerList:")
	for _, server := range strings.Split(s, ",") {
		// TODO: STUN / TURN url format validation?
		log.Println(server)
		option := webrtc.OptionIceServer(server)
		*i = append(*i, option)
	}
	return nil
}

type BytesLogger interface {
	Log()
	AddOutbound(int)
	AddInbound(int)
}

// Default BytesLogger does nothing.
type BytesNullLogger struct{}

func (b BytesNullLogger) Log()                   {}
func (b BytesNullLogger) AddOutbound(amount int) {}
func (b BytesNullLogger) AddInbound(amount int)  {}

// BytesSyncLogger uses channels to safely log from multiple sources with output
// occuring at reasonable intervals.
type BytesSyncLogger struct {
	outboundChan chan int
	inboundChan  chan int
	outbound     int
	inbound      int
	outEvents    int
	inEvents     int
	isLogging    bool
}

func (b *BytesSyncLogger) Log() {
	b.isLogging = true
	var amount int
	output := func() {
		log.Printf("Traffic Bytes (in|out): %d | %d -- (%d OnMessages, %d Sends)",
			b.inbound, b.outbound, b.inEvents, b.outEvents)
		b.outbound = 0
		b.outEvents = 0
		b.inbound = 0
		b.inEvents = 0
	}
	last := time.Now()
	for {
		select {
		case amount = <-b.outboundChan:
			b.outbound += amount
			b.outEvents++
			last := time.Now()
			if time.Since(last) > time.Second*LogTimeInterval {
				last = time.Now()
				output()
			}
		case amount = <-b.inboundChan:
			b.inbound += amount
			b.inEvents++
			if time.Since(last) > time.Second*LogTimeInterval {
				last = time.Now()
				output()
			}
		case <-time.After(time.Second * LogTimeInterval):
			if b.inEvents > 0 || b.outEvents > 0 {
				output()
			}
		}
	}
}

func (b *BytesSyncLogger) AddOutbound(amount int) {
	if !b.isLogging {
		return
	}
	b.outboundChan <- amount
}

func (b *BytesSyncLogger) AddInbound(amount int) {
	if !b.isLogging {
		return
	}
	b.inboundChan <- amount
}
