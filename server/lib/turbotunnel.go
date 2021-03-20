package lib

import (
	"sync"

	"git.torproject.org/pluggable-transports/snowflake.git/common/turbotunnel"
)

// clientIDMap is a fixed-capacity mapping from ClientIDs to address strings.
// Adding a new entry using the Set method causes the oldest existing entry to
// be forgotten.
//
// This data type is meant to be used to remember the IP address associated with
// a ClientID, during the short period of time between when a WebSocket
// connection with that ClientID began, and when a KCP session is established.
//
// The design requirements of this type are that it needs to remember a mapping
// for only a short time, and old entries should expire so as not to consume
// unbounded memory. It is not a critical error if an entry is forgotten before
// it is needed; better to forget entries than to use too much memory.
type clientIDMap struct {
	lock sync.Mutex
	// entries is a circular buffer of (ClientID, addr) pairs.
	entries []struct {
		clientID turbotunnel.ClientID
		addr     string
	}
	// oldest is the index of the oldest member of the entries buffer, the
	// one that will be overwritten at the next call to Set.
	oldest int
	// current points to the index of the most recent entry corresponding to
	// each ClientID.
	current map[turbotunnel.ClientID]int
}

// newClientIDMap makes a new clientIDMap with the given capacity.
func newClientIDMap(capacity int) *clientIDMap {
	return &clientIDMap{
		entries: make([]struct {
			clientID turbotunnel.ClientID
			addr     string
		}, capacity),
		oldest:  0,
		current: make(map[turbotunnel.ClientID]int),
	}
}

// Set adds a mapping from clientID to addr, replacing any previous mapping for
// clientID. It may also cause the clientIDMap to forget at most one other
// mapping, the oldest one.
func (m *clientIDMap) Set(clientID turbotunnel.ClientID, addr string) {
	m.lock.Lock()
	defer m.lock.Unlock()
	if len(m.entries) == 0 {
		// The invariant m.oldest < len(m.entries) does not hold in this
		// special case.
		return
	}
	// m.oldest is the index of the entry we're about to overwrite. If it's
	// the current entry for any ClientID, we need to delete that clientID
	// from the current map (that ClientID is now forgotten).
	if i, ok := m.current[m.entries[m.oldest].clientID]; ok && i == m.oldest {
		delete(m.current, m.entries[m.oldest].clientID)
	}
	// Overwrite the oldest entry.
	m.entries[m.oldest].clientID = clientID
	m.entries[m.oldest].addr = addr
	// Add the overwritten entry to the quick-lookup map.
	m.current[clientID] = m.oldest
	// What was the oldest entry is now the newest.
	m.oldest = (m.oldest + 1) % len(m.entries)
}

// Get returns a previously stored mapping. The second return value indicates
// whether clientID was actually present in the map. If it is false, then the
// returned address string will be "".
func (m *clientIDMap) Get(clientID turbotunnel.ClientID) (string, bool) {
	m.lock.Lock()
	defer m.lock.Unlock()
	if i, ok := m.current[clientID]; ok {
		return m.entries[i].addr, true
	} else {
		return "", false
	}
}
