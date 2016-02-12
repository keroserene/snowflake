package snowflake_broker

import (
	"container/heap"
	"testing"
)

func TestSnowflakeHeap(t *testing.T) {
	h := new(SnowflakeHeap)
	heap.Init(h)
	if 0 != h.Len() {
		t.Error("Unexpected length.")
	}
	s1 := new(Snowflake)
	s2 := new(Snowflake)
	s3 := new(Snowflake)
	s4 := new(Snowflake)

	s1.clients = 4
	s2.clients = 5
	s3.clients = 3
	s4.clients = 1

	heap.Push(h, s1)
	if 1 != h.Len() {
	}
	heap.Push(h, s2)
	heap.Push(h, s3)
	heap.Push(h, s4)

	if 4 != h.Len() {
		t.Error("Unexpected length.")
	}

	heap.Remove(h, 0)
	if 3 != h.Len() {
		t.Error("Unexpected length.")
	}

	r := heap.Pop(h).(*Snowflake)
	if 2 != h.Len() {
		t.Error("Unexpected length.")
	}
	if r.clients != 3 {
		t.Error("Unexpected clients: ", r.clients)
	}
	if r.index != -1 {
		t.Error("Unexpected index: ", r.index)
	}

	r = heap.Pop(h).(*Snowflake)
	if 1 != h.Len() {
		t.Error("Unexpected length.")
	}
	if r.clients != 4 {
		t.Error("Unexpected clients: ", r.clients)
	}

	r = heap.Pop(h).(*Snowflake)
	if r.clients != 5 {
		t.Error("Unexpected clients: ", r.clients)
	}

	if 0 != h.Len() {
		t.Error("Unexpected length.")
	}
}
