package eventloop

import (
	"context"
	"reflect"
	"sync"
	"time"
)

// EventHandler processes the given event. It should return true if the event is consumed.
type EventHandler func(event interface{}) (consume bool)

// EventLoop synchronously executes a queue of events.
type EventLoop struct {
	mut sync.Mutex

	eventQ        chan interface{}
	waitingEvents map[interface{}][]interface{}

	handlers      map[reflect.Type]EventHandler
	asyncHandlers map[reflect.Type][]EventHandler
	tickers       map[int]*ticker
	tickerID      int
}

// New returns a new event loop with the requested buffer size.
func New(bufferSize uint) *EventLoop {
	el := &EventLoop{
		eventQ:        make(chan interface{}, bufferSize),
		waitingEvents: make(map[interface{}][]interface{}),
		handlers:      make(map[reflect.Type]EventHandler),
		asyncHandlers: make(map[reflect.Type][]EventHandler),
		tickers:       make(map[int]*ticker),
	}
	return el
}

// RegisterHandler registers a handler for events with the same type as the 'eventType' argument.
// The handler is executed synchronously. There can only be one synchronous handler for each type.
func (el *EventLoop) RegisterHandler(eventType interface{}, handler EventHandler) {
	t := reflect.TypeOf(eventType)
	el.handlers[t] = handler
}

// RegisterAsyncHandler registers a handler for events with the same type as the 'eventType' argument.
// The handler is executed asynchronously, immediately after a new event arrives.
// There can be multiple async handlers per type.
// If the handler returns true, it will prevent any other handlers from getting the event,
// and the event will not be added to the event queue.
func (el *EventLoop) RegisterAsyncHandler(eventType interface{}, handler EventHandler) {
	t := reflect.TypeOf(eventType)
	el.asyncHandlers[t] = append(el.asyncHandlers[t], handler)
}

// AddEvent adds an event to the event queue.
//
// The event may be processed or consumed by an async handler before it enters the queue.
// It is not safe to call this function from the the event loop goroutine.
// If you need to send add an event from a handler, use a goroutine:
//  go EventLoop.AddEvent(...)
func (el *EventLoop) AddEvent(event interface{}) {
	for _, handler := range el.asyncHandlers[reflect.TypeOf(event)] {
		if handler(event) {
			return // event was consumed
		}
	}

	el.eventQ <- event
}

// Run runs the event loop. A context object can be provided to stop the event loop.
func (el *EventLoop) Run(ctx context.Context) {
	for {
		select {
		case event := <-el.eventQ:
			if e, ok := event.(startTickerEvent); ok {
				el.startTicker(ctx, e.tickerID)
				break
			}
			el.processEvent(event)
		case <-ctx.Done():
			goto cancelled
		}
	}

cancelled:
	// HACK: when we get cancelled, we will handle the events that were in the queue at that time before quitting.
	l := len(el.eventQ)
	for i := 0; i < l; i++ {
		el.processEvent(<-el.eventQ)
	}
}

// processEvent dispatches the event to the correct handler.
func (el *EventLoop) processEvent(e interface{}) {
	if f, ok := e.(func()); ok {
		f()
	} else {
		handler := el.handlers[reflect.TypeOf(e)]
		if handler != nil {
			handler(e)
		}
	}

	el.mut.Lock()
	for k, v := range el.waitingEvents {
		if reflect.TypeOf(e) == reflect.TypeOf(k) {
			// must use a goroutine to avoid deadlock
			go func(events []interface{}) {
				for _, event := range v {
					el.AddEvent(event)
				}
			}(v)
			delete(el.waitingEvents, k)
		}
	}
	el.mut.Unlock()
}

// DelayUntil allows us to delay handling of an event until after another event has happened.
// The eventType parameter decides the type of event to wait for, and it should be the zero value
// of that event type. The event parameter is the event that will be delayed.
func (el *EventLoop) DelayUntil(eventType, event interface{}) {
	el.mut.Lock()
	v := el.waitingEvents[eventType]
	v = append(v, event)
	el.waitingEvents[eventType] = v
	el.mut.Unlock()
}

type ticker struct {
	interval time.Duration
	event    interface{}
	cancel   context.CancelFunc
}

type startTickerEvent struct {
	tickerID int
}

// AddTicker adds a ticker with the specified interval and returns the ticker id.
// The ticker will send the specified event on the event loop at regular intervals.
// The returned ticker id can be used to remove the ticker with RemoveTicker.
// The ticker will not be started before the event loop is running.
func (el *EventLoop) AddTicker(interval time.Duration, event interface{}) int {
	el.mut.Lock()

	id := el.tickerID
	el.tickerID++

	ticker := ticker{
		interval: interval,
		event:    event,
		cancel:   func() {}, // initialized to empty function to avoid nil
	}
	el.tickers[id] = &ticker

	el.mut.Unlock()

	el.eventQ <- startTickerEvent{id}

	return id
}

// RemoveTicker removes the ticker with the specified id.
// If the ticker was removed, RemoveTicker will return true.
// If the ticker does not exist, false will be returned instead.
func (el *EventLoop) RemoveTicker(id int) bool {
	el.mut.Lock()
	defer el.mut.Unlock()

	ticker, ok := el.tickers[id]
	if !ok {
		return false
	}
	ticker.cancel()
	delete(el.tickers, id)
	return true
}

func (el *EventLoop) startTicker(ctx context.Context, id int) {
	// lock the mutex such that the ticker cannot be removed until we have started it
	el.mut.Lock()
	defer el.mut.Unlock()
	ticker, ok := el.tickers[id]
	if !ok {
		return
	}
	ctx, ticker.cancel = context.WithCancel(ctx)
	go el.runTicker(ctx, ticker)
}

func (el *EventLoop) runTicker(ctx context.Context, ticker *ticker) {
	t := time.NewTicker(ticker.interval)
	for {
		select {
		case <-t.C:
			el.AddEvent(ticker.event)
		case <-ctx.Done():
			t.Stop()
			return
		}
	}
}
