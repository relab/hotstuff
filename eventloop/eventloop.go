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

// EventLoop accepts events of any type and executes registered event handlers for pipelines.
type EventLoop struct {
	eventQ queue

	mut sync.Mutex // protects the following:

	ctx context.Context // set by Run

	waitingEvents map[reflect.Type][]any

	handlers map[pipelinedReflectTypeKey][]handler

	tickers  map[pipelinedTickerKey]*ticker
	tickerID int
}

type pipelinedReflectTypeKey struct {
	pipelineId    pipelining.PipelineId
	reflectedType reflect.Type
}

type pipelinedTickerKey struct {
	pipelineId pipelining.PipelineId
	tickerId   int
}

// New returns a new event loop with the requested buffer size.
func New(bufferSize uint) *EventLoop {
	el := &EventLoop{
		ctx:           context.Background(),
		eventQ:        newQueue(bufferSize),
		waitingEvents: make(map[reflect.Type][]any),
		handlers:      make(map[pipelinedReflectTypeKey][]handler),
		tickers:       make(map[pipelinedTickerKey]*ticker),
	}
	return el
}

type pipelinedEventHandle struct {
	pipelineId pipelining.PipelineId
	event      any
}

// RegisterObserver registers a handler with priority.
// Deprecated: use RegisterHandler and the Prioritize option instead.
func (el *EventLoop) RegisterObserver(eventType any, handler EventHandler) int {
	return el.registerPipelinedHandler(pipelining.PipelineIdNone, eventType, []HandlerOption{Prioritize()}, handler)
}

// UnregisterObserver unregister a handler.
// Deprecated: use UnregisterHandler instead.
func (el *EventLoop) UnregisterObserver(eventType any, id int) {
	el.UnregisterPipelinedHandler(pipelining.PipelineIdNone, eventType, id)
}

// RegisterHandler registers the given event handler for the given event type with the given handler options, if any.
// If no handler options are provided, the default handler options will be used.
func (el *EventLoop) RegisterHandler(eventType any, handler EventHandler, opts ...HandlerOption) int {
	return el.registerPipelinedHandler(pipelining.PipelineIdNone, eventType, opts, handler)
}

// RegisterPipelinedObserver registers a handler with priority.
// Deprecated: use RegisterHandler and the Prioritize option instead.
func (el *EventLoop) RegisterPipelinedObserver(pipelineId pipelining.PipelineId, eventType any, handler EventHandler) int {
	return el.registerPipelinedHandler(pipelineId, eventType, []HandlerOption{Prioritize()}, handler)
}

// UnregisterPipelinedObserver unregister a handler.
// Deprecated: use UnregisterHandler instead.
func (el *EventLoop) UnregisterPipelinedObserver(pipelineId pipelining.PipelineId, eventType any, id int) {
	el.UnregisterPipelinedHandler(pipelineId, eventType, id)
}

// RegisterPipelinedHandler registers the given event handler for the given event type with the given handler options, if any.
// If no handler options are provided, the default handler options will be used.
func (el *EventLoop) RegisterPipelinedHandler(pipelineId pipelining.PipelineId, eventType any, handler EventHandler, opts ...HandlerOption) int {
	return el.registerPipelinedHandler(pipelineId, eventType, opts, handler)
}

func (el *EventLoop) registerPipelinedHandler(pipelineId pipelining.PipelineId, eventType any, opts []HandlerOption, callback EventHandler) int {
	h := handler{callback: callback}

	for _, opt := range opts {
		opt(&h.opts)
	}

	el.mut.Lock()
	defer el.mut.Unlock()
	t := reflect.TypeOf(eventType)

	key := pipelinedReflectTypeKey{
		pipelineId:    pipelineId,
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

	// Because of maps with separate lists of handlers, where pipelineID is key, the i (id)
	// of two or more handler lists may overlap. What uniquely identifies a handler list is
	// i + pipelineId
	// TODO: Figure out if IDs need to be unique
	return i
}

func (el *EventLoop) UnregisterHandler(eventType any, id int) {
	el.UnregisterPipelinedHandler(pipelining.PipelineIdNone, eventType, id)
}

// UnregisterPipelinedHandler unregisters the handler for the given event type with the given id.
func (el *EventLoop) UnregisterPipelinedHandler(pipelineId pipelining.PipelineId, eventType any, id int) {
	el.mut.Lock()
	defer el.mut.Unlock()
	t := reflect.TypeOf(eventType)
	key := pipelinedReflectTypeKey{
		pipelineId:    pipelineId,
		reflectedType: t,
	}
	el.handlers[key][id].callback = nil
}

func (el *EventLoop) AddEvent(event any) {
	el.AddPipelinedEvent(pipelining.PipelineIdNone, event)
}

// AddPipelinedEvent adds an event to the event queue.
func (el *EventLoop) AddPipelinedEvent(pipelineID pipelining.PipelineId, event any) {
	if event != nil {
		handle := pipelinedEventHandle{
			pipelineId: pipelineID,
			event:      event,
		}
		// run handlers with runInAddEvent option
		el.processEvent(pipelineID, event, true)
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
		handle := event.(pipelinedEventHandle)
		if e, ok := handle.event.(startTickerEvent); ok {
			el.startPipelinedTicker(handle.pipelineId, e.tickerID)
			continue
		}
		el.processEvent(handle.pipelineId, handle.event, false)
	}

	// HACK: when we get canceled, we will handle the events that were in the queue at that time before quitting.
	l := el.eventQ.len()
	for i := 0; i < l; i++ {
		event, _ := el.eventQ.pop()
		handle := event.(pipelinedEventHandle)
		el.processEvent(handle.pipelineId, handle.event, false)
	}
}

// Tick processes a single event. Returns true if an event was handled.
func (el *EventLoop) Tick(ctx context.Context) bool {
	el.setContext(ctx)

	event, ok := el.eventQ.pop()
	if !ok {
		return false
	}

	handle := event.(pipelinedEventHandle)
	if e, ok := handle.event.(startTickerEvent); ok {
		el.startPipelinedTicker(handle.pipelineId, e.tickerID)
	} else {
		el.processEvent(handle.pipelineId, handle.event, false)
	}

	return true
}

var pipelinedHandlerListPool = gpool.New(func() []EventHandler { return make([]EventHandler, 0, 10) })

// processEvent dispatches the event to the correct handler.
func (el *EventLoop) processEvent(pipelineId pipelining.PipelineId, event any, runningInAddEvent bool) {
	t := reflect.TypeOf(event)

	if !runningInAddEvent {
		defer el.dispatchDelayedEvents(pipelineId, t)
	}

	// Must copy handlers to a list so that they can be executed after unlocking the mutex.
	// This looks like it might be slow, but there should be few handlers (< 10) registered for each event type.
	// We use a pool to reduce memory allocations.
	priorityList := pipelinedHandlerListPool.Get()
	handlerList := pipelinedHandlerListPool.Get()

	el.mut.Lock()
	key := pipelinedReflectTypeKey{
		pipelineId:    pipelineId,
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
	pipelinedHandlerListPool.Put(priorityList)

	for _, handler := range handlerList {
		handler(event)
	}

	handlerList = handlerList[:0]
	pipelinedHandlerListPool.Put(handlerList)
}

func (el *EventLoop) dispatchDelayedEvents(pipelineID pipelining.PipelineId, t reflect.Type) {
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
		el.AddPipelinedEvent(pipelineID, event)
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
	return el.AddPipelinedTicker(pipelining.PipelineIdNone, interval, callback)
}

func (el *EventLoop) RemoveTicker(id int) bool {
	return el.RemovePipelinedTicker(pipelining.PipelineIdNone, id)
}

// AddPipelinedTicker adds a ticker with the specified interval and returns the ticker id.
// The ticker will send the specified event on the event loop at regular intervals.
// The returned ticker id can be used to remove the ticker with RemoveTicker.
// The ticker will not be started before the event loop is running.
func (el *EventLoop) AddPipelinedTicker(pipelineId pipelining.PipelineId, interval time.Duration, callback func(tick time.Time) (event any)) int {
	el.mut.Lock()

	id := el.tickerID
	el.tickerID++

	key := pipelinedTickerKey{
		pipelineId: pipelineId,
		tickerId:   id,
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
	el.eventQ.push(pipelinedEventHandle{
		pipelineId: pipelineId,
		event:      startTickerEvent{id},
	})

	return id
}

// RemovePipelinedTicker removes the ticker with the specified id.
// If the ticker was removed, RemovePipelinedTicker will return true.
// If the ticker does not exist, false will be returned instead.
func (el *EventLoop) RemovePipelinedTicker(pipelineId pipelining.PipelineId, id int) bool {
	el.mut.Lock()
	defer el.mut.Unlock()
	key := pipelinedTickerKey{
		pipelineId: pipelineId,
		tickerId:   id,
	}

	// TODO: Check if the pipeline exists too
	ticker, ok := el.tickers[key]
	if !ok {
		return false
	}
	ticker.cancel()
	delete(el.tickers, key)
	return true
}

func (el *EventLoop) startPipelinedTicker(pipelineId pipelining.PipelineId, id int) {
	// lock the mutex such that the ticker cannot be removed until we have started it
	el.mut.Lock()
	defer el.mut.Unlock()
	key := pipelinedTickerKey{
		pipelineId: pipelineId,
		tickerId:   id,
	}
	ticker, ok := el.tickers[key]
	if !ok {
		return
	}
	ctx := el.ctx
	ctx, ticker.cancel = context.WithCancel(ctx)
	go el.runPipelinedTicker(pipelineId, ctx, ticker)
}

func (el *EventLoop) runPipelinedTicker(pipelineId pipelining.PipelineId, ctx context.Context, ticker *ticker) {
	t := time.NewTicker(ticker.interval)
	defer t.Stop()

	if ctx.Err() != nil {
		return
	}

	// send the first event immediately
	// TODO: Find out if this is correct
	el.AddPipelinedEvent(pipelineId, ticker.callback(time.Now()))

	for {
		select {
		case tick := <-t.C:
			el.AddPipelinedEvent(pipelineId, ticker.callback(tick))
		case <-ctx.Done():
			return
		}
	}
}
