package event

import "sync"

func NewSnowflakeEventDispatcher() SnowflakeEventDispatcher {
	return &eventBus{lock: &sync.Mutex{}}
}

type eventBus struct {
	lock      *sync.Mutex
	listeners []SnowflakeEventReceiver
}

func (e *eventBus) OnNewSnowflakeEvent(event SnowflakeEvent) {
	e.lock.Lock()
	defer e.lock.Unlock()
	for _, v := range e.listeners {
		v.OnNewSnowflakeEvent(event)
	}
}

func (e *eventBus) AddSnowflakeEventListener(receiver SnowflakeEventReceiver) {
	e.lock.Lock()
	defer e.lock.Unlock()
	e.listeners = append(e.listeners, receiver)
}

func (e *eventBus) RemoveSnowflakeEventListener(receiver SnowflakeEventReceiver) {
	e.lock.Lock()
	defer e.lock.Unlock()
	var newListeners []SnowflakeEventReceiver
	for _, v := range e.listeners {
		if v != receiver {
			newListeners = append(newListeners, v)
		}
	}
	e.listeners = newListeners
	return
}
