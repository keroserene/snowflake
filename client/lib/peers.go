package snowflake_client

import (
	"container/list"
	"errors"
	"fmt"
	"log"
	"sync"
)

// Peers is a container that keeps track of multiple WebRTC remote peers.
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
	bytesLogger bytesLogger

	snowflakeChan chan *WebRTCPeer
	activePeers   *list.List

	melt chan struct{}

	collectLock sync.Mutex
}

// NewPeers constructs a fresh container of remote peers.
func NewPeers(tongue Tongue) (*Peers, error) {
	p := &Peers{}
	// Use buffered go channel to pass snowflakes onwards to the SOCKS handler.
	if tongue == nil {
		return nil, errors.New("missing Tongue to catch Snowflakes with")
	}
	p.snowflakeChan = make(chan *WebRTCPeer, tongue.GetMax())
	p.activePeers = list.New()
	p.melt = make(chan struct{})
	p.Tongue = tongue
	return p, nil
}

// Collect connects to and adds a new remote peer as part of |SnowflakeCollector| interface.
func (p *Peers) Collect() (*WebRTCPeer, error) {
	// Engage the Snowflake Catching interface, which must be available.
	p.collectLock.Lock()
	defer p.collectLock.Unlock()
	select {
	case <-p.melt:
		return nil, fmt.Errorf("Snowflakes have melted")
	default:
	}
	if nil == p.Tongue {
		return nil, errors.New("missing Tongue to catch Snowflakes with")
	}
	cnt := p.Count()
	capacity := p.Tongue.GetMax()
	s := fmt.Sprintf("Currently at [%d/%d]", cnt, capacity)
	if cnt >= capacity {
		return nil, fmt.Errorf("At capacity [%d/%d]", cnt, capacity)
	}
	log.Println("WebRTC: Collecting a new Snowflake.", s)
	// BUG: some broker conflict here.
	connection, err := p.Tongue.Catch()
	if nil != err {
		return nil, err
	}
	// Track new valid Snowflake in internal collection and pass along.
	p.activePeers.PushBack(connection)
	p.snowflakeChan <- connection
	return connection, nil
}

// Pop blocks until an available, valid snowflake appears.
// Pop will return nil after End has been called.
func (p *Peers) Pop() *WebRTCPeer {
	for {
		snowflake, ok := <-p.snowflakeChan
		if !ok {
			return nil
		}
		if snowflake.Closed() {
			continue
		}
		// Set to use the same rate-limited traffic logger to keep consistency.
		snowflake.bytesLogger = p.bytesLogger
		return snowflake
	}
}

// Melted returns a channel that will close when peers stop being collected.
// Melted is a necessary part of |SnowflakeCollector| interface.
func (p *Peers) Melted() <-chan struct{} {
	return p.melt
}

// Count returns the total available Snowflakes (including the active ones)
// The count only reduces when connections themselves close, rather than when
// they are popped.
func (p *Peers) Count() int {
	p.purgeClosedPeers()
	return p.activePeers.Len()
}

func (p *Peers) purgeClosedPeers() {
	for e := p.activePeers.Front(); e != nil; {
		next := e.Next()
		conn := e.Value.(*WebRTCPeer)
		// Purge those marked for deletion.
		if conn.Closed() {
			p.activePeers.Remove(e)
		}
		e = next
	}
}

// End closes all active connections to Peers contained here, and stops the
// collection of future Peers.
func (p *Peers) End() {
	close(p.melt)
	p.collectLock.Lock()
	defer p.collectLock.Unlock()
	close(p.snowflakeChan)
	cnt := p.Count()
	for e := p.activePeers.Front(); e != nil; {
		next := e.Next()
		conn := e.Value.(*WebRTCPeer)
		conn.Close()
		p.activePeers.Remove(e)
		e = next
	}
	log.Printf("WebRTC: melted all %d snowflakes.", cnt)
}
