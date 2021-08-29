// Package eventloop provides an event loop which is widely used by modules.
//
// The event loop allows for flexible handling of events through the concept of observers and handlers.
// An observer is a function that is able to view an event before it is handled.
// Thus, there can be multiple observers for each event type.
// A handler is a function that processes the event. There can only be one handler for each event type.
//
// The event loop also allows observers/handlers to run at two different times:
//
// 1. When an event is added (concurrent with the event loop).
// 2. When an event has passed through the event queue.
//
// The former allows for concurrent handling of events, as multiple goroutines can be adding events concurrently,
// which means that the observers or handlers can run concurrently.
// The latter makes it possible to write handlers that will run on the same goroutine, one at a time.
package eventloop

import (
	"context"
	"reflect"
	"sync"
	"time"
)

// EventHandler processes an event.
type EventHandler func(event interface{})

type handlerDesc struct {
	async  bool
	handle EventHandler
}

// EventLoop accepts events of any type and executes relevant event handlers.
// It supports registering both observers and handlers based on the type of event that they accept.
// The difference between them is that there can be many observers per event type, but only one handler.
// Handlers and observers can either be executed asynchronously, immediately after AddEvent() is called,
// or synchronously, after the event has passed through the event queue.
// An asynchronous handler consumes events, which means that synchronous observers will not be notified of them.
type EventLoop struct {
	mut sync.Mutex

	eventQ        chan interface{}
	waitingEvents map[interface{}][]interface{}

	handlers  map[reflect.Type]handlerDesc
	observers map[reflect.Type][]handlerDesc

	tickers  map[int]*ticker
	tickerID int
}

// New returns a new event loop with the requested buffer size.
func New(bufferSize uint) *EventLoop {
	el := &EventLoop{
		eventQ:        make(chan interface{}, bufferSize),
		waitingEvents: make(map[interface{}][]interface{}),
		handlers:      make(map[reflect.Type]handlerDesc),
		observers:     make(map[reflect.Type][]handlerDesc),
		tickers:       make(map[int]*ticker),
	}
	return el
}

// RegisterHandler registers a handler for events with the same type as the 'eventType' argument.
// The handler is executed synchronously. There can be only one handler per event type.
func (el *EventLoop) RegisterHandler(eventType interface{}, handler EventHandler) {
	el.handlers[reflect.TypeOf(eventType)] = handlerDesc{
		async:  false,
		handle: handler,
	}
}

// RegisterAsyncHandler registers a handler for events with the same type as the 'eventType' argument.
// The handler is executed asynchronously, immediately after a new event arrives.
// The handler consumes the event, which means that it will not be observed by synchronous observers.
// There can be only one handler per event type.
func (el *EventLoop) RegisterAsyncHandler(eventType interface{}, handler EventHandler) {
	el.handlers[reflect.TypeOf(eventType)] = handlerDesc{
		async:  true,
		handle: handler,
	}
}

// RegisterObserver registers an observer for events with the same type as the 'eventType' argument.
// The observer is executed synchronously before any registered handler.
func (el *EventLoop) RegisterObserver(eventType interface{}, observer EventHandler) {
	t := reflect.TypeOf(eventType)
	el.observers[t] = append(el.observers[t], handlerDesc{
		async:  false,
		handle: observer,
	})
}

// RegisterAsyncObserver registers an observer for events with the same type as the 'eventType' argument.
// The observer is executed asynchronously before any registered handler.
func (el *EventLoop) RegisterAsyncObserver(eventType interface{}, observer EventHandler) {
	t := reflect.TypeOf(eventType)
	el.observers[t] = append(el.observers[t], handlerDesc{
		async:  true,
		handle: observer,
	})
}

// AddEvent adds an event to the event queue.
//
// The event may be processed or consumed by an async handler before it enters the queue.
// It is not safe to call this function from the the event loop goroutine.
// If you need to send add an event from a handler, use a goroutine:
//  go EventLoop.AddEvent(...)
func (el *EventLoop) AddEvent(event interface{}) {
	t := reflect.TypeOf(event)
	// run async observers
	for _, observer := range el.observers[t] {
		if observer.async {
			observer.handle(event)
		}
	}
	// run async handler
	if handler, ok := el.handlers[t]; ok && handler.async {
		handler.handle(event)
		return
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
func (el *EventLoop) processEvent(event interface{}) {
	t := reflect.TypeOf(event)
	defer el.dispatchDelayedEvents(t)

	if f, ok := event.(func()); ok {
		f()
		return
	}

	// run observers
	for _, observer := range el.observers[t] {
		if !observer.async {
			observer.handle(event)
		}
	}

	if handler, ok := el.handlers[t]; ok && !handler.async {
		handler.handle(event)
	}
}

func (el *EventLoop) dispatchDelayedEvents(t reflect.Type) {

	el.mut.Lock()
	if delayed, ok := el.waitingEvents[t]; ok {
		// must use a goroutine to avoid deadlock
		go func(events []interface{}) {
			for _, event := range delayed {
				el.AddEvent(event)
			}
		}(delayed)
		delete(el.waitingEvents, t)
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
	callback func(time.Time) interface{}
	cancel   context.CancelFunc
}

type startTickerEvent struct {
	tickerID int
}

// AddTicker adds a ticker with the specified interval and returns the ticker id.
// The ticker will send the specified event on the event loop at regular intervals.
// The returned ticker id can be used to remove the ticker with RemoveTicker.
// The ticker will not be started before the event loop is running.
func (el *EventLoop) AddTicker(interval time.Duration, callback func(tick time.Time) (event interface{})) int {
	el.mut.Lock()

	id := el.tickerID
	el.tickerID++

	ticker := ticker{
		interval: interval,
		callback: callback,
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
	defer t.Stop()

	if ctx.Err() != nil {
		return
	}

	// send the first event immediately
	el.AddEvent(ticker.callback(time.Now()))

	for {
		select {
		case tick := <-t.C:
			el.AddEvent(ticker.callback(tick))
		case <-ctx.Done():
			return
		}
	}
}
