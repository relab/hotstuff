package waitutil

import (
	"context"
	"sync"
	"time"
)

// WaitUtil allows multiple goroutines to wait for something to happen
type WaitUtil struct {
	mut sync.Mutex
	c   chan struct{}
}

// NewWaitUtil returns a new instance of WaitUtil
func NewWaitUtil() *WaitUtil {
	return &WaitUtil{
		c: make(chan struct{}),
	}
}

// WaitContext waits for the context to be done or for a signal. Returns true if context is done, false otherwise.
func (w *WaitUtil) WaitContext(ctx context.Context) bool {
	w.mut.Lock()
	_chan := w.c
	w.mut.Unlock()

	select {
	case <-ctx.Done():
		return true
	case <-_chan:
		return false
	}
}

// WaitCondTimeout waits for either a condition to be true, or for a timeout to elsapse.
func (w *WaitUtil) WaitCondTimeout(timeout time.Duration, cond func() bool) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	for {
		if cond() || w.WaitContext(ctx) {
			break
		}
	}
	cancel()
}

// WakeAll wakes all waiting goroutines
func (w *WaitUtil) WakeAll() {
	w.mut.Lock()
	close(w.c)
	w.c = make(chan struct{})
	w.mut.Unlock()
}
