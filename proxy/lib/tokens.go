package snowflake_proxy

import (
	"sync/atomic"
)

type tokens_t struct {
	ch       chan struct{}
	capacity uint
	clients  int64
}

func newTokens(capacity uint) *tokens_t {
	var ch chan struct{}
	if capacity != 0 {
		ch = make(chan struct{}, capacity)
	}

	return &tokens_t{
		ch:       ch,
		capacity: capacity,
		clients:  0,
	}
}

func (t *tokens_t) get() {
	atomic.AddInt64(&t.clients, 1)

	if t.capacity != 0 {
		t.ch <- struct{}{}
	}
}

func (t *tokens_t) ret() {
	atomic.AddInt64(&t.clients, -1)

	if t.capacity != 0 {
		<-t.ch
	}
}

func (t tokens_t) count() int64 {
	return atomic.LoadInt64(&t.clients)
}
