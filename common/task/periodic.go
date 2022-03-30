// Package task
// Reused from https://github.com/v2fly/v2ray-core/blob/784775f68922f07d40c9eead63015b2026af2ade/common/task/periodic.go
/*
The MIT License (MIT)

Copyright (c) 2015-2021 V2Ray & V2Fly Community

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/
package task

import (
	"sync"
	"time"
)

// Periodic is a task that runs periodically.
type Periodic struct {
	// Interval of the task being run
	Interval time.Duration
	// Execute is the task function
	Execute func() error

	access  sync.Mutex
	timer   *time.Timer
	running bool
}

func (t *Periodic) hasClosed() bool {
	t.access.Lock()
	defer t.access.Unlock()

	return !t.running
}

func (t *Periodic) checkedExecute() error {
	if t.hasClosed() {
		return nil
	}

	if err := t.Execute(); err != nil {
		t.access.Lock()
		t.running = false
		t.access.Unlock()
		return err
	}

	t.access.Lock()
	defer t.access.Unlock()

	if !t.running {
		return nil
	}

	t.timer = time.AfterFunc(t.Interval, func() {
		t.checkedExecute()
	})

	return nil
}

// Start implements common.Runnable.
func (t *Periodic) Start() error {
	t.access.Lock()
	if t.running {
		t.access.Unlock()
		return nil
	}
	t.running = true
	t.access.Unlock()

	if err := t.checkedExecute(); err != nil {
		t.access.Lock()
		t.running = false
		t.access.Unlock()
		return err
	}

	return nil
}

func (t *Periodic) WaitThenStart() {
	time.AfterFunc(t.Interval, func() {
		t.Start()
	})
}

// Close implements common.Closable.
func (t *Periodic) Close() error {
	t.access.Lock()
	defer t.access.Unlock()

	t.running = false
	if t.timer != nil {
		t.timer.Stop()
		t.timer = nil
	}

	return nil
}
