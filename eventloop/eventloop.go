// Package eventloop provides an event loop which is widely used by modules.
//
// The event loop allows for flexible handling of events through the concept of observers and handlers.
// An observer is a function that is able to view an event before it is handled.
// Thus, there can be multiple observers for each event type.
// A handler is a function that processes the event. There can only be one handler for each event type.
package eventloop

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/relab/hotstuff/util/gpool"
)

type handlerOpts struct {
	async    bool
	priority bool
}

// HandlerOption sets configuration options for event handlers.
type HandlerOption func(*handlerOpts)

// RunAsync instructs the eventloop to run the handler asynchronously.
func RunAsync() HandlerOption {
	return func(ho *handlerOpts) {
		ho.async = true
	}
}

// WithPriority instructs the eventloop to prioritize running the handler before others.
// This guarantees that the handler runs before handlers that have not requested priority.
func WithPriority() HandlerOption {
	return func(ho *handlerOpts) {
		ho.priority = true
	}
}

// EventHandler processes an event.
type EventHandler func(event any)

type handler struct {
	callback EventHandler
	opts     handlerOpts
}

// EventLoop accepts events of any type and executes relevant event handlers.
// It supports registering both observers and handlers based on the type of event that they accept.
// The difference between them is that there can be many observers per event type, but only one handler,
// and the handler is executed last.
type EventLoop struct {
	eventQ queue

	mut sync.Mutex // protects the following:

	ctx context.Context // set by Run

	waitingEvents map[reflect.Type][]any

	handlers map[reflect.Type][]handler

	tickers  map[int]*ticker
	tickerID int
}

// New returns a new event loop with the requested buffer size.
func New(bufferSize uint) *EventLoop {
	el := &EventLoop{
		ctx:           context.Background(),
		eventQ:        newQueue(bufferSize),
		waitingEvents: make(map[reflect.Type][]any),
		handlers:      make(map[reflect.Type][]handler),
		tickers:       make(map[int]*ticker),
	}
	return el
}

// RegisterObserver registers a handler with priority.
// Deprecated: use RegisterHandler and the WithPriority option instead.
func (el *EventLoop) RegisterObserver(eventType any, handler EventHandler) int {
	return el.RegisterHandler(eventType, handler, WithPriority())
}

// UnregisterObserver unregister a handler.
// Deprecated: use UnregisterHandler instead.
func (el *EventLoop) UnregisterObserver(eventType any, id int) {
	el.UnregisterHandler(eventType, id)
}

// RegisterHandler registers an event handler. The handler will
func (el *EventLoop) RegisterHandler(eventType any, callback EventHandler, opts ...HandlerOption) int {
	h := handler{callback: callback}

	for _, opt := range opts {
		opt(&h.opts)
	}

	el.mut.Lock()
	defer el.mut.Unlock()
	t := reflect.TypeOf(eventType)

	handlers := el.handlers[t]

	i := 0
	for ; i < len(handlers); i++ {
		if handlers[i].callback == nil {
			break
		}
	}

	if i == len(handlers) {
		handlers = append(handlers, h)
	} else {
		handlers[i] = h
	}

	el.handlers[t] = handlers

	return i
}

// UnregisterHandler unregisters the handler for the given event type with the given id.
func (el *EventLoop) UnregisterHandler(eventType any, id int) {
	el.mut.Lock()
	defer el.mut.Unlock()
	t := reflect.TypeOf(eventType)
	el.handlers[t][id].callback = nil
}

// AddEvent adds an event to the event queue.
func (el *EventLoop) AddEvent(event any) {
	if event != nil {
		el.eventQ.push(event)
		el.processEvent(event, true)
	}
}

// Context returns the context associated with the event loop.
// Usually, this context will be the one passed to Run.
// However, if Tick is used instead of Run, Context will return
// the last context that was passed to Tick.
// If neither Run nor Tick have been called,
// Context returns context.Background.
func (el *EventLoop) Context() context.Context {
	el.mut.Lock()
	defer el.mut.Unlock()

	return el.ctx
}

func (el *EventLoop) setContext(ctx context.Context) {
	el.mut.Lock()
	defer el.mut.Unlock()

	el.ctx = ctx
}

// Run runs the event loop. A context object can be provided to stop the event loop.
func (el *EventLoop) Run(ctx context.Context) {
	el.setContext(ctx)

loop:
	for {
		event, ok := el.eventQ.pop()
		if !ok {
			select {
			case <-el.eventQ.ready():
				continue loop
			case <-ctx.Done():
				break loop
			}
		}
		if e, ok := event.(startTickerEvent); ok {
			el.startTicker(ctx, e.tickerID)
			continue
		}
		el.processEvent(event, false)
	}

	// HACK: when we get cancelled, we will handle the events that were in the queue at that time before quitting.
	l := el.eventQ.len()
	for i := 0; i < l; i++ {
		event, _ := el.eventQ.pop()
		el.processEvent(event, false)
	}
}

// Tick processes a single event. Returns true if an event was handled.
func (el *EventLoop) Tick(ctx context.Context) bool {
	el.setContext(ctx)

	event, ok := el.eventQ.pop()
	if !ok {
		return false
	}

	if e, ok := event.(startTickerEvent); ok {
		el.startTicker(context.Background(), e.tickerID)
	} else {
		el.processEvent(event, false)
	}

	return true
}

var handlerListPool = gpool.New(func() []EventHandler { return make([]EventHandler, 0, 10) })

// processEvent dispatches the event to the correct handler.
func (el *EventLoop) processEvent(event any, async bool) {
	t := reflect.TypeOf(event)

	if !async {
		defer el.dispatchDelayedEvents(t)
	}

	if f, ok := event.(func()); ok {
		f()
		return
	}

	// Must copy handlers to a list so that they can be executed after unlocking the mutex.
	// Use a pool to reduce memory allocations.
	priorityList := handlerListPool.Get()
	handlerList := handlerListPool.Get()

	el.mut.Lock()
	for _, handler := range el.handlers[t] {
		if handler.opts.async != async || handler.callback == nil {
			continue
		}
		if handler.opts.priority {
			priorityList = append(priorityList, handler.callback)
		} else {
			handlerList = append(handlerList, handler.callback)
		}
	}
	el.mut.Unlock()

	for _, handler := range priorityList {
		handler(event)
	}

	priorityList = priorityList[:0]
	handlerListPool.Put(priorityList)

	for _, handler := range handlerList {
		handler(event)
	}

	handlerList = handlerList[:0]
	handlerListPool.Put(handlerList)
}

func (el *EventLoop) dispatchDelayedEvents(t reflect.Type) {
	var (
		events []any
		ok     bool
	)

	el.mut.Lock()
	if events, ok = el.waitingEvents[t]; ok {
		delete(el.waitingEvents, t)
	}
	el.mut.Unlock()

	for _, event := range events {
		el.AddEvent(event)
	}
}

// DelayUntil allows us to delay handling of an event until after another event has happened.
// The eventType parameter decides the type of event to wait for, and it should be the zero value
// of that event type. The event parameter is the event that will be delayed.
func (el *EventLoop) DelayUntil(eventType, event any) {
	if eventType == nil || event == nil {
		return
	}
	el.mut.Lock()
	t := reflect.TypeOf(eventType)
	v := el.waitingEvents[t]
	v = append(v, event)
	el.waitingEvents[t] = v
	el.mut.Unlock()
}

type ticker struct {
	interval time.Duration
	callback func(time.Time) any
	cancel   context.CancelFunc
}

type startTickerEvent struct {
	tickerID int
}

// AddTicker adds a ticker with the specified interval and returns the ticker id.
// The ticker will send the specified event on the event loop at regular intervals.
// The returned ticker id can be used to remove the ticker with RemoveTicker.
// The ticker will not be started before the event loop is running.
func (el *EventLoop) AddTicker(interval time.Duration, callback func(tick time.Time) (event any)) int {
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

	// We want the ticker to inherit the context of the event loop,
	// so we need to start the ticker from the run loop.
	el.eventQ.push(startTickerEvent{id})

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
