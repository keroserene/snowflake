package turbotunnel

import (
	"container/heap"
	"net"
	"sync"
	"time"
)

// clientRecord is a record of a recently seen client, with the time it was last
// seen and a send queue.
type clientRecord struct {
	Addr      net.Addr
	LastSeen  time.Time
	SendQueue chan []byte
}

// ClientMap manages a mapping of live clients (keyed by address, which will be
// a ClientID) to their respective send queues. ClientMap's functions are safe
// to call from multiple goroutines.
type ClientMap struct {
	// We use an inner structure to avoid exposing public heap.Interface
	// functions to users of clientMap.
	inner clientMapInner
	// Synchronizes access to inner.
	lock sync.Mutex
}

// NewClientMap creates a ClientMap that expires clients after a timeout.
//
// The timeout does not have to be kept in sync with QUIC's internal idle
// timeout. If a client is removed from the client map while the QUIC session is
// still live, the worst that can happen is a loss of whatever packets were in
// the send queue at the time. If QUIC later decides to send more packets to the
// same client, we'll instantiate a new send queue, and if the client ever
// connects again with the proper client ID, we'll deliver them.
func NewClientMap(timeout time.Duration) *ClientMap {
	m := &ClientMap{
		inner: clientMapInner{
			byAge:  make([]*clientRecord, 0),
			byAddr: make(map[net.Addr]int),
		},
	}
	go func() {
		for {
			time.Sleep(timeout / 2)
			now := time.Now()
			m.lock.Lock()
			m.inner.removeExpired(now, timeout)
			m.lock.Unlock()
		}
	}()
	return m
}

// SendQueue returns the send queue corresponding to addr, creating it if
// necessary.
func (m *ClientMap) SendQueue(addr net.Addr) chan []byte {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.inner.SendQueue(addr, time.Now())
}

// clientMapInner is the inner type of ClientMap, implementing heap.Interface.
// byAge is the backing store, a heap ordered by LastSeen time, to facilitate
// expiring old client records. byAddr is a map from addresses (i.e., ClientIDs)
// to heap indices, to allow looking up by address. Unlike ClientMap,
// clientMapInner requires external synchonization.
type clientMapInner struct {
	byAge  []*clientRecord
	byAddr map[net.Addr]int
}

// removeExpired removes all client records whose LastSeen timestamp is more
// than timeout in the past.
func (inner *clientMapInner) removeExpired(now time.Time, timeout time.Duration) {
	for len(inner.byAge) > 0 && now.Sub(inner.byAge[0].LastSeen) >= timeout {
		heap.Pop(inner)
	}
}

// SendQueue finds the existing client record corresponding to addr, or creates
// a new one if none exists yet. It updates the client record's LastSeen time
// and returns its SendQueue.
func (inner *clientMapInner) SendQueue(addr net.Addr, now time.Time) chan []byte {
	var record *clientRecord
	i, ok := inner.byAddr[addr]
	if ok {
		// Found one, update its LastSeen.
		record = inner.byAge[i]
		record.LastSeen = now
		heap.Fix(inner, i)
	} else {
		// Not found, create a new one.
		record = &clientRecord{
			Addr:      addr,
			LastSeen:  now,
			SendQueue: make(chan []byte, queueSize),
		}
		heap.Push(inner, record)
	}
	return record.SendQueue
}

// heap.Interface for clientMapInner.

func (inner *clientMapInner) Len() int {
	if len(inner.byAge) != len(inner.byAddr) {
		panic("inconsistent clientMap")
	}
	return len(inner.byAge)
}

func (inner *clientMapInner) Less(i, j int) bool {
	return inner.byAge[i].LastSeen.Before(inner.byAge[j].LastSeen)
}

func (inner *clientMapInner) Swap(i, j int) {
	inner.byAge[i], inner.byAge[j] = inner.byAge[j], inner.byAge[i]
	inner.byAddr[inner.byAge[i].Addr] = i
	inner.byAddr[inner.byAge[j].Addr] = j
}

func (inner *clientMapInner) Push(x interface{}) {
	record := x.(*clientRecord)
	if _, ok := inner.byAddr[record.Addr]; ok {
		panic("duplicate address in clientMap")
	}
	// Insert into byAddr map.
	inner.byAddr[record.Addr] = len(inner.byAge)
	// Insert into byAge slice.
	inner.byAge = append(inner.byAge, record)
}

func (inner *clientMapInner) Pop() interface{} {
	n := len(inner.byAddr)
	// Remove from byAge slice.
	record := inner.byAge[n-1]
	inner.byAge[n-1] = nil
	inner.byAge = inner.byAge[:n-1]
	// Remove from byAddr map.
	delete(inner.byAddr, record.Addr)
	close(record.SendQueue)
	return record
}
