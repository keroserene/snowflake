package event

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

type stubReceiver struct {
	counter int
}

func (s *stubReceiver) OnNewSnowflakeEvent(event SnowflakeEvent) {
	s.counter++
}

func TestBusDispatch(t *testing.T) {
	EventBus := NewSnowflakeEventDispatcher()
	StubReceiverA := &stubReceiver{}
	StubReceiverB := &stubReceiver{}
	EventBus.AddSnowflakeEventListener(StubReceiverA)
	EventBus.AddSnowflakeEventListener(StubReceiverB)
	assert.Equal(t, 0, StubReceiverA.counter)
	assert.Equal(t, 0, StubReceiverB.counter)
	EventBus.OnNewSnowflakeEvent(EventOnSnowflakeConnected{})
	assert.Equal(t, 1, StubReceiverA.counter)
	assert.Equal(t, 1, StubReceiverB.counter)
	EventBus.RemoveSnowflakeEventListener(StubReceiverB)
	EventBus.OnNewSnowflakeEvent(EventOnSnowflakeConnected{})
	assert.Equal(t, 2, StubReceiverA.counter)
	assert.Equal(t, 1, StubReceiverB.counter)

}
