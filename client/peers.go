package main

import (
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
	current       *webRTCConn
	capacity      int
	// TODO: Probably not necessary.
	maxedChan chan struct{}
}

// Construct a fresh container of remote peers.
func NewPeers(max int) *Peers {
	p := &Peers{capacity: max, current: nil}
	// Use buffered go channel to pass new snowflakes onwards to the SOCKS handler.
	p.snowflakeChan = make(chan *webRTCConn, max)
	p.maxedChan = make(chan struct{}, 1)
	return p
}

// TODO: Needs fixing.
func (p *Peers) Count() int {
	count := 0
	if p.current != nil {
		count = 1
	}
	return count + len(p.snowflakeChan)
}

// As part of |SnowflakeCollector| interface.
func (p *Peers) Collect() error {
	if p.Count() >= p.capacity {
		s := fmt.Sprintf("At capacity [%d/%d]", p.Count(), p.capacity)
		p.maxedChan <- struct{}{}
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
  // Use the same rate-limited traffic logger to keep consistency.
	connection.BytesLogger = p.BytesLogger
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
	p.current = snowflake
	snowflake.BytesLogger = p.BytesLogger
	return snowflake
}

// Close all remote peers.
func (p *Peers) End() {
	log.Printf("WebRTC: interruped")
	if nil != p.current {
		p.current.Close()
	}
	for r := range p.snowflakeChan {
		r.Close()
	}
}
