package eventloop

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/relab/hotstuff/pipelining"
	"github.com/relab/hotstuff/util/gpool"
)

// PipelinedEventLoop accepts events of any type and executes registered event handlers for pipelines.
type PipelinedEventLoop struct {
	eventQ queue

	mut sync.Mutex // protects the following:

	ctx context.Context // set by Run

	waitingEvents map[reflect.Type][]any

	handlers map[reflect.Type]map[pipelining.PipelineId][]handler

	tickers  map[int]*ticker
	tickerID int
}

// New returns a new event loop with the requested buffer size.
func NewPipelined(bufferSize uint) *PipelinedEventLoop {
	el := &PipelinedEventLoop{
		ctx:           context.Background(),
		eventQ:        newQueue(bufferSize),
		waitingEvents: make(map[reflect.Type][]any),
		handlers:      make(map[reflect.Type]map[pipelining.PipelineId][]handler),
		tickers:       make(map[int]*ticker),
	}
	return el
}

type pipelinedEventHandle struct {
	pipelineId pipelining.PipelineId
	event      any
}

// RegisterObserver registers a handler with priority.
// Deprecated: use RegisterHandler and the Prioritize option instead.
func (el *PipelinedEventLoop) RegisterObserver(eventType any, handler EventHandler) int {
	// return el.registerHandler(eventType, []HandlerOption{Prioritize()}, handler)
	// TODO: Implement
	panic("not implemented")
}

// UnregisterObserver unregister a handler.
// Deprecated: use UnregisterHandler instead.
func (el *PipelinedEventLoop) UnregisterObserver(eventType any, id int) {
	// el.UnregisterHandler(eventType, id)
	// TODO: Implement
	panic("not implemented")
}

// RegisterHandler registers the given event handler for the given event type with the given handler options, if any.
// If no handler options are provided, the default handler options will be used.
func (el *PipelinedEventLoop) RegisterHandler(pipelineId pipelining.PipelineId, eventType any, handler EventHandler, opts ...HandlerOption) int {
	return el.registerHandler(pipelineId, eventType, opts, handler)
}

func (el *PipelinedEventLoop) registerHandler(pipelineId pipelining.PipelineId, eventType any, opts []HandlerOption, callback EventHandler) int {
	h := handler{callback: callback}

	for _, opt := range opts {
		opt(&h.opts)
	}

	el.mut.Lock()
	defer el.mut.Unlock()
	t := reflect.TypeOf(eventType)

	_, ok := el.handlers[t]
	if !ok {
		el.handlers[t] = make(map[pipelining.PipelineId][]handler)
	}

	handlers := el.handlers[t]

	// _, ok = handlers[pipelineId]
	// if !ok {
	// 	handlers[pipelineId] = make([]handler, 0)
	// }

	// search for a free slot for the handler
	i := 0
	for ; i < len(handlers); i++ {
		if handlers[pipelineId][i].callback == nil {
			break
		}
	}

	// no free slots; have to grow the list
	if i == len(handlers) {
		handlers[pipelineId] = append(handlers[pipelineId], h)
	} else {
		handlers[pipelineId][i] = h
	}

	el.handlers[t] = handlers

	// Because of maps with separate lists of handlers, where pipelineID is key, the i (id)
	// of two or more handler lists may overlap. What uniquely identifies a handler list is
	// i + pipelineId
	// TODO: Figure out if IDs need to be unique
	return i
}

// UnregisterHandler unregisters the handler for the given event type with the given id.
func (el *PipelinedEventLoop) UnregisterHandler(pipelineId pipelining.PipelineId, eventType any, id int) {
	el.mut.Lock()
	defer el.mut.Unlock()
	t := reflect.TypeOf(eventType)
	el.handlers[t][pipelineId][id].callback = nil
}

// AddEvent adds an event to the event queue.
func (el *PipelinedEventLoop) AddEvent(pipelineID pipelining.PipelineId, event any) {
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
func (el *PipelinedEventLoop) Context() context.Context {
	el.mut.Lock()
	defer el.mut.Unlock()

	return el.ctx
}

func (el *PipelinedEventLoop) setContext(ctx context.Context) {
	el.mut.Lock()
	defer el.mut.Unlock()

	el.ctx = ctx
}

// Run runs the event loop. A context object can be provided to stop the event loop.
func (el *PipelinedEventLoop) Run(ctx context.Context) {
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
			el.startTicker(e.tickerID)
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
func (el *PipelinedEventLoop) Tick(ctx context.Context) bool {
	el.setContext(ctx)

	event, ok := el.eventQ.pop()
	if !ok {
		return false
	}

	handle := event.(pipelinedEventHandle)
	if e, ok := handle.event.(startTickerEvent); ok {
		el.startTicker(e.tickerID)
	} else {
		el.processEvent(handle.pipelineId, handle.event, false)
	}

	return true
}

var pipelinedHandlerListPool = gpool.New(func() []EventHandler { return make([]EventHandler, 0, 10) })

// processEvent dispatches the event to the correct handler.
func (el *PipelinedEventLoop) processEvent(pipelineID pipelining.PipelineId, event any, runningInAddEvent bool) {
	t := reflect.TypeOf(event)

	if !runningInAddEvent {
		defer el.dispatchDelayedEvents(pipelineID, t)
	}

	// Must copy handlers to a list so that they can be executed after unlocking the mutex.
	// This looks like it might be slow, but there should be few handlers (< 10) registered for each event type.
	// We use a pool to reduce memory allocations.
	priorityList := pipelinedHandlerListPool.Get()
	handlerList := pipelinedHandlerListPool.Get()

	el.mut.Lock()
	for _, handler := range el.handlers[t][pipelineID] {
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

func (el *PipelinedEventLoop) dispatchDelayedEvents(pipelineID pipelining.PipelineId, t reflect.Type) {
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
		el.AddEvent(pipelineID, event)
	}
}

// DelayUntil allows us to delay handling of an event until after another event has happened.
// The eventType parameter decides the type of event to wait for, and it should be the zero value
// of that event type. The event parameter is the event that will be delayed.
func (el *PipelinedEventLoop) DelayUntil(eventType, event any) {
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

// AddTicker adds a ticker with the specified interval and returns the ticker id.
// The ticker will send the specified event on the event loop at regular intervals.
// The returned ticker id can be used to remove the ticker with RemoveTicker.
// The ticker will not be started before the event loop is running.
func (el *PipelinedEventLoop) AddTicker(interval time.Duration, callback func(tick time.Time) (event any)) int {
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
func (el *PipelinedEventLoop) RemoveTicker(id int) bool {
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

func (el *PipelinedEventLoop) startTicker(id int) {
	// lock the mutex such that the ticker cannot be removed until we have started it
	el.mut.Lock()
	defer el.mut.Unlock()
	ticker, ok := el.tickers[id]
	if !ok {
		return
	}
	ctx := el.ctx
	ctx, ticker.cancel = context.WithCancel(ctx)
	go el.runTicker(ctx, ticker)
}

func (el *PipelinedEventLoop) runTicker(ctx context.Context, ticker *ticker) {
	t := time.NewTicker(ticker.interval)
	defer t.Stop()

	if ctx.Err() != nil {
		return
	}

	// send the first event immediately
	// TODO: Find out if this is correct
	el.AddEvent(pipelining.PipelineIdNone, ticker.callback(time.Now()))

	for {
		select {
		case tick := <-t.C:
			el.AddEvent(pipelining.PipelineIdNone, ticker.callback(tick))
		case <-ctx.Done():
			return
		}
	}
}
