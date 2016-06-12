package main

import (
	"container/list"
	"errors"
	"fmt"
	"log"
)

// Container which keeps track of multiple WebRTC remote peers.
// Implements |SnowflakeCollector|.
//
// Maintaining a set of pre-connected Peers with fresh but inactive datachannels
// allows allows rapid recovery when the current WebRTC Peer disconnects.
//
// Note: For now, only one remote can be active at any given moment.
// This is a property of Tor circuits & its current multiplexing constraints,
// but could be updated if that changes.
// (Also, this constraint does not necessarily apply to the more generic PT
// version of Snowflake)
type Peers struct {
	Tongue
	BytesLogger

	snowflakeChan chan *webRTCConn
	activePeers   *list.List
	capacity      int
}

// Construct a fresh container of remote peers.
func NewPeers(max int) *Peers {
	p := &Peers{capacity: max}
	// Use buffered go channel to pass new snowflakes onwards to the SOCKS handler.
	p.snowflakeChan = make(chan *webRTCConn, max)
	p.activePeers = list.New()
	return p
}

// As part of |SnowflakeCollector| interface.
func (p *Peers) Collect() error {
	cnt := p.Count()
	if cnt >= p.capacity {
		s := fmt.Sprintf("At capacity [%d/%d]", cnt, p.capacity)
		return errors.New(s)
	}
	// Engage the Snowflake Catching interface, which must be available.
	if nil == p.Tongue {
		return errors.New("Missing Tongue to catch Snowflakes with.")
	}
	connection, err := p.Tongue.Catch()
	if nil == connection || nil != err {
		return err
	}
	// Track new valid Snowflake in internal collection and pass along.
	p.activePeers.PushBack(connection)
	p.snowflakeChan <- connection
	return nil
}

// As part of |SnowflakeCollector| interface.
func (p *Peers) Pop() *webRTCConn {
	// Blocks until an available snowflake appears.
	snowflake, ok := <-p.snowflakeChan
	if !ok {
		return nil
	}
	// Set to use the same rate-limited traffic logger to keep consistency.
	snowflake.BytesLogger = p.BytesLogger
	return snowflake
}

// Returns total available Snowflakes (including the active one)
// The count only reduces when connections themselves close, rather than when
// they are popped.
func (p *Peers) Count() int {
	p.purgeClosedPeers()
	return p.activePeers.Len()
}

func (p *Peers) purgeClosedPeers() {
	for e := p.activePeers.Front(); e != nil; {
		next := e.Next()
		conn := e.Value.(*webRTCConn)
		// Purge those marked for deletion.
		if conn.closed {
			p.activePeers.Remove(e)
		}
		e = next
	}
}

// Close all Peers contained here.
func (p *Peers) End() {
	log.Printf("WebRTC: Ending all peer connections.")
	for e := p.activePeers.Front(); e != nil; e = e.Next() {
		conn := e.Value.(*webRTCConn)
		conn.Close()
	}
}
