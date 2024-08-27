package eventloop

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/relab/hotstuff/pipelining"
	"github.com/relab/hotstuff/util/gpool"
)

type handlerOpts struct {
	runInAddEvent bool
	priority      bool
}

// HandlerOption sets configuration options for event handlers.
type HandlerOption func(*handlerOpts)

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
	callback EventHandler
	opts     handlerOpts
}

// Prioritize instructs the event loop to run the handler before handlers that do not have priority.
// It should only be used if you must look at an event before other handlers get to look at it.
func Prioritize() HandlerOption {
	return func(ho *handlerOpts) {
		ho.priority = true
	}
}

// UnsafeRunInAddEvent instructs the eventloop to run the handler as a part of AddEvent.
// Handlers that use this option can process events before they are added to the event queue.
// Because AddEvent could be running outside the event loop, it is unsafe.
// Only thread-safe modules can be used safely from a handler using this option.
func UnsafeRunInAddEvent() HandlerOption {
	return func(ho *handlerOpts) {
		ho.runInAddEvent = true
	}
}

// EventLoop accepts events of any type and executes registered event handlers for pipes.
type EventLoop struct {
	eventQ queue

	mut sync.Mutex // protects the following:

	ctx context.Context // set by Run

	waitingEvents map[reflect.Type][]any

	handlers map[pipedReflectTypeKey][]handler

	tickers  map[pipedTickerKey]*ticker
	tickerID int
}

type pipedReflectTypeKey struct {
	pipeId        pipelining.PipeId
	reflectedType reflect.Type
}

type pipedTickerKey struct {
	pipeId   pipelining.PipeId
	tickerId int
}

// New returns a new event loop with the requested buffer size.
func New(bufferSize uint) *EventLoop {
	el := &EventLoop{
		ctx:           context.Background(),
		eventQ:        newQueue(bufferSize),
		waitingEvents: make(map[reflect.Type][]any),
		handlers:      make(map[pipedReflectTypeKey][]handler),
		tickers:       make(map[pipedTickerKey]*ticker),
	}
	return el
}

type pipedEventHandle struct {
	pipeId pipelining.PipeId
	event  any
}

// RegisterObserver registers a handler with priority.
// Deprecated: use RegisterHandler and the Prioritize option instead.
func (el *EventLoop) RegisterObserver(eventType any, handler EventHandler) int {
	return el.registerPipedHandler(pipelining.NoPipeId, eventType, []HandlerOption{Prioritize()}, handler)
}

// UnregisterObserver unregister a handler.
// Deprecated: use UnregisterHandler instead.
func (el *EventLoop) UnregisterObserver(eventType any, id int) {
	el.UnregisterPipedHandler(pipelining.NoPipeId, eventType, id)
}

// RegisterHandler registers the given event handler for the given event type with the given handler options, if any.
// If no handler options are provided, the default handler options will be used.
func (el *EventLoop) RegisterHandler(eventType any, handler EventHandler, opts ...HandlerOption) int {
	return el.registerPipedHandler(pipelining.NoPipeId, eventType, opts, handler)
}

// RegisterPipedObserver registers a handler with priority.
// Deprecated: use RegisterHandler and the Prioritize option instead.
func (el *EventLoop) RegisterPipedObserver(pipeId pipelining.PipeId, eventType any, handler EventHandler) int {
	return el.registerPipedHandler(pipeId, eventType, []HandlerOption{Prioritize()}, handler)
}

// UnregisterPipedObserver unregister a handler.
// Deprecated: use UnregisterHandler instead.
func (el *EventLoop) UnregisterPipedObserver(pipeId pipelining.PipeId, eventType any, id int) {
	el.UnregisterPipedHandler(pipeId, eventType, id)
}

// RegisterPipedHandler registers the given event handler for the given event type with the given handler options, if any.
// If no handler options are provided, the default handler options will be used.
func (el *EventLoop) RegisterPipedHandler(pipeId pipelining.PipeId, eventType any, handler EventHandler, opts ...HandlerOption) int {
	return el.registerPipedHandler(pipeId, eventType, opts, handler)
}

func (el *EventLoop) registerPipedHandler(pipeId pipelining.PipeId, eventType any, opts []HandlerOption, callback EventHandler) int {
	h := handler{callback: callback}

	for _, opt := range opts {
		opt(&h.opts)
	}

	el.mut.Lock()
	defer el.mut.Unlock()
	t := reflect.TypeOf(eventType)

	key := pipedReflectTypeKey{
		pipeId:        pipeId,
		reflectedType: t,
	}

	handlers := el.handlers[key]

	// search for a free slot for the handler
	i := 0
	for ; i < len(handlers); i++ {
		if handlers[i].callback == nil {
			break
		}
	}

	// no free slots; have to grow the list
	if i == len(handlers) {
		handlers = append(handlers, h)
	} else {
		handlers[i] = h
	}

	el.handlers[key] = handlers

	// Because of maps with separate lists of handlers, where pipeID is key, the i (id)
	// of two or more handler lists may overlap. What uniquely identifies a handler list is
	// i + pipeId
	// TODO: Figure out if IDs need to be unique
	return i
}

func (el *EventLoop) UnregisterHandler(eventType any, id int) {
	el.UnregisterPipedHandler(pipelining.NoPipeId, eventType, id)
}

// UnregisterPipedHandler unregisters the handler for the given event type with the given id.
func (el *EventLoop) UnregisterPipedHandler(pipeId pipelining.PipeId, eventType any, id int) {
	el.mut.Lock()
	defer el.mut.Unlock()
	t := reflect.TypeOf(eventType)
	key := pipedReflectTypeKey{
		pipeId:        pipeId,
		reflectedType: t,
	}
	el.handlers[key][id].callback = nil
}

func (el *EventLoop) AddEvent(event any) {
	el.PipeEvent(pipelining.NoPipeId, event)
}

// PipeEvent adds an event to the event pipe.
// TODO: Parallellize the piped events. They are added to the same synchronous queue now.
func (el *EventLoop) PipeEvent(pipeID pipelining.PipeId, event any) {
	if event != nil {
		handle := pipedEventHandle{
			pipeId: pipeID,
			event:  event,
		}
		// run handlers with runInAddEvent option
		el.processEvent(pipeID, event, true)
		el.eventQ.push(handle)
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
		handle := event.(pipedEventHandle)
		if e, ok := handle.event.(startTickerEvent); ok {
			el.startPipedTicker(handle.pipeId, e.tickerID)
			continue
		}
		el.processEvent(handle.pipeId, handle.event, false)
	}

	// HACK: when we get canceled, we will handle the events that were in the queue at that time before quitting.
	l := el.eventQ.len()
	for i := 0; i < l; i++ {
		event, _ := el.eventQ.pop()
		handle := event.(pipedEventHandle)
		el.processEvent(handle.pipeId, handle.event, false)
	}
}

// Tick processes a single event. Returns true if an event was handled.
func (el *EventLoop) Tick(ctx context.Context) bool {
	el.setContext(ctx)

	event, ok := el.eventQ.pop()
	if !ok {
		return false
	}

	handle := event.(pipedEventHandle)
	if e, ok := handle.event.(startTickerEvent); ok {
		el.startPipedTicker(handle.pipeId, e.tickerID)
	} else {
		el.processEvent(handle.pipeId, handle.event, false)
	}

	return true
}

var pipedHandlerListPool = gpool.New(func() []EventHandler { return make([]EventHandler, 0, 10) })

// processEvent dispatches the event to the correct handler.
func (el *EventLoop) processEvent(pipeId pipelining.PipeId, event any, runningInAddEvent bool) {
	t := reflect.TypeOf(event)

	if !runningInAddEvent {
		defer el.dispatchDelayedEvents(pipeId, t)
	}

	// Must copy handlers to a list so that they can be executed after unlocking the mutex.
	// This looks like it might be slow, but there should be few handlers (< 10) registered for each event type.
	// We use a pool to reduce memory allocations.
	priorityList := pipedHandlerListPool.Get()
	handlerList := pipedHandlerListPool.Get()

	el.mut.Lock()
	key := pipedReflectTypeKey{
		pipeId:        pipeId,
		reflectedType: t,
	}
	for _, handler := range el.handlers[key] {
		if handler.opts.runInAddEvent != runningInAddEvent || handler.callback == nil {
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
	pipedHandlerListPool.Put(priorityList)

	for _, handler := range handlerList {
		handler(event)
	}

	handlerList = handlerList[:0]
	pipedHandlerListPool.Put(handlerList)
}

func (el *EventLoop) dispatchDelayedEvents(pipeID pipelining.PipeId, t reflect.Type) {
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
		el.PipeEvent(pipeID, event)
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

func (el *EventLoop) AddTicker(interval time.Duration, callback func(tick time.Time) (event any)) int {
	return el.AddPipedTicker(pipelining.NoPipeId, interval, callback)
}

func (el *EventLoop) RemoveTicker(id int) bool {
	return el.RemovePipedTicker(pipelining.NoPipeId, id)
}

// AddPipedTicker adds a ticker with the specified interval and returns the ticker id.
// The ticker will send the specified event on the event loop at regular intervals.
// The returned ticker id can be used to remove the ticker with RemoveTicker.
// The ticker will not be started before the event loop is running.
func (el *EventLoop) AddPipedTicker(pipeId pipelining.PipeId, interval time.Duration, callback func(tick time.Time) (event any)) int {
	el.mut.Lock()

	id := el.tickerID
	el.tickerID++

	key := pipedTickerKey{
		pipeId:   pipeId,
		tickerId: id,
	}

	ticker := ticker{
		interval: interval,
		callback: callback,
		cancel:   func() {}, // initialized to empty function to avoid nil
	}

	el.tickers[key] = &ticker

	el.mut.Unlock()

	// We want the ticker to inherit the context of the event loop,
	// so we need to start the ticker from the run loop.
	el.eventQ.push(pipedEventHandle{
		pipeId: pipeId,
		event:  startTickerEvent{id},
	})

	return id
}

// RemovePipedTicker removes the ticker with the specified id.
// If the ticker was removed, RemovePipedTicker will return true.
// If the ticker does not exist, false will be returned instead.
func (el *EventLoop) RemovePipedTicker(pipeId pipelining.PipeId, id int) bool {
	el.mut.Lock()
	defer el.mut.Unlock()
	key := pipedTickerKey{
		pipeId:   pipeId,
		tickerId: id,
	}

	// TODO: Check if the pipe exists too
	ticker, ok := el.tickers[key]
	if !ok {
		return false
	}
	ticker.cancel()
	delete(el.tickers, key)
	return true
}

func (el *EventLoop) startPipedTicker(pipeId pipelining.PipeId, id int) {
	// lock the mutex such that the ticker cannot be removed until we have started it
	el.mut.Lock()
	defer el.mut.Unlock()
	key := pipedTickerKey{
		pipeId:   pipeId,
		tickerId: id,
	}
	ticker, ok := el.tickers[key]
	if !ok {
		return
	}
	ctx := el.ctx
	ctx, ticker.cancel = context.WithCancel(ctx)
	go el.runPipedTicker(pipeId, ctx, ticker)
}

func (el *EventLoop) runPipedTicker(pipeId pipelining.PipeId, ctx context.Context, ticker *ticker) {
	t := time.NewTicker(ticker.interval)
	defer t.Stop()

	if ctx.Err() != nil {
		return
	}

	// send the first event immediately
	// TODO: Find out if this is correct
	el.PipeEvent(pipeId, ticker.callback(time.Now()))

	for {
		select {
		case tick := <-t.C:
			el.PipeEvent(pipeId, ticker.callback(tick))
		case <-ctx.Done():
			return
		}
	}
}
