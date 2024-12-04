package eventloop

import (
	"context"
	"reflect"
	"time"

	"github.com/relab/hotstuff"
)

type ticker struct {
	interval time.Duration
	callback func(time.Time) any
	cancel   context.CancelFunc
}

type startTickerEvent struct {
	tickerID int
}

// EventHandler processes an event.
type EventHandler func(event any)

type handler struct {
	callback  EventHandler
	opts      handlerOpts
	eventType reflect.Type
}

type handlerOpts struct {
	runInAddEvent bool
	priority      bool
	pipe          hotstuff.Pipe
}

// HandlerOption sets configuration options for event handlers.
type HandlerOption func(*handlerOpts)

// UnsafeRunInAddEvent instructs the eventloop to run the handler as a part of AddEvent.
// Handlers that use this option can process events before they are added to the event queue.
// Because AddEvent could be running outside the event loop, it is unsafe.
// Only thread-safe modules can be used safely from a handler using this option.
func UnsafeRunInAddEvent() HandlerOption {
	return func(ho *handlerOpts) {
		ho.runInAddEvent = true
	}
}

// Prioritize instructs the event loop to run the handler before handlers that do not have priority.
// It should only be used if you must look at an event before other handlers get to look at it.
func Prioritize() HandlerOption {
	return func(ho *handlerOpts) {
		ho.priority = true
	}
}

// RespondToScope assigns which pipe (scope) to respond to when ScopeEvent is used to add an event to the
// eventloop. If the NullPipe (0) is passed, this handler option will not take effect.
func RespondToScope(pipe hotstuff.Pipe) HandlerOption {
	return func(ho *handlerOpts) {
		ho.pipe = pipe
	}
}
