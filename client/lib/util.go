package lib

import (
	"encoding/json"
	"log"
	"time"

	"github.com/pion/webrtc/v2"
)

const (
	LogTimeInterval = 5
)

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
	OutboundChan chan int
	InboundChan  chan int
	Outbound     int
	Inbound      int
	OutEvents    int
	InEvents     int
	IsLogging    bool
}

func (b *BytesSyncLogger) Log() {
	b.IsLogging = true
	var amount int
	output := func() {
		log.Printf("Traffic Bytes (in|out): %d | %d -- (%d OnMessages, %d Sends)",
			b.Inbound, b.Outbound, b.InEvents, b.OutEvents)
		b.Outbound = 0
		b.OutEvents = 0
		b.Inbound = 0
		b.InEvents = 0
	}
	last := time.Now()
	for {
		select {
		case amount = <-b.OutboundChan:
			b.Outbound += amount
			b.OutEvents++
			if time.Since(last) > time.Second*LogTimeInterval {
				last = time.Now()
				output()
			}
		case amount = <-b.InboundChan:
			b.Inbound += amount
			b.InEvents++
			if time.Since(last) > time.Second*LogTimeInterval {
				last = time.Now()
				output()
			}
		case <-time.After(time.Second * LogTimeInterval):
			if b.InEvents > 0 || b.OutEvents > 0 {
				output()
			}
		}
	}
}

func (b *BytesSyncLogger) AddOutbound(amount int) {
	if !b.IsLogging {
		return
	}
	b.OutboundChan <- amount
}

func (b *BytesSyncLogger) AddInbound(amount int) {
	if !b.IsLogging {
		return
	}
	b.InboundChan <- amount
}
func deserializeSessionDescription(msg string) *webrtc.SessionDescription {
	var parsed map[string]interface{}
	err := json.Unmarshal([]byte(msg), &parsed)
	if nil != err {
		log.Println(err)
		return nil
	}
	if _, ok := parsed["type"]; !ok {
		log.Println("Cannot deserialize SessionDescription without type field.")
		return nil
	}
	if _, ok := parsed["sdp"]; !ok {
		log.Println("Cannot deserialize SessionDescription without sdp field.")
		return nil
	}

	var stype webrtc.SDPType
	switch parsed["type"].(string) {
	default:
		log.Println("Unknown SDP type")
		return nil
	case "offer":
		stype = webrtc.SDPTypeOffer
	case "pranswer":
		stype = webrtc.SDPTypePranswer
	case "answer":
		stype = webrtc.SDPTypeAnswer
	case "rollback":
		stype = webrtc.SDPTypeRollback
	}

	if err != nil {
		log.Println(err)
		return nil
	}
	return &webrtc.SessionDescription{
		Type: stype,
		SDP:  parsed["sdp"].(string),
	}
}

func serializeSessionDescription(desc *webrtc.SessionDescription) string {
	bytes, err := json.Marshal(*desc)
	if nil != err {
		log.Println(err)
		return ""
	}
	return string(bytes)
}
