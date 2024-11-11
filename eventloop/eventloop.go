// Package eventloop provides an event loop which is widely used by modules.
// The use of the event loop enables many of the modules to run synchronously, thus removing the need for thread safety.
// This simplifies the implementation of modules and reduces the risks of race conditions.
//
// The event loop can accept events of any type.
// It uses reflection to determine what handler function to execute based on the type of an event.
package eventloop

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/util/gpool"
)

type handlerOpts struct {
	runInAddEvent     bool
	priority          bool
	consensusInstance hotstuff.Instance
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

// RespondToInstance assigns which consensus instance to respond to when PipeEvent is used to add an event to the
// eventloop. If the NullPipeId (0) is passed, this handler option will not take effect.
func RespondToInstance(instance hotstuff.Instance) HandlerOption {
	return func(ho *handlerOpts) {
		ho.consensusInstance = instance
	}
}

type pipedEventWrapper struct {
	consensusInstance hotstuff.Instance
	event             any
}

// EventHandler processes an event.
type EventHandler func(event any)

type handler struct {
	callback  EventHandler
	opts      handlerOpts
	eventType reflect.Type
}

// EventLoop accepts events of any type and executes registered event handlers.
type EventLoop struct {
	eventQ queue

	mut sync.Mutex // protects the following:

	ctx context.Context // set by Run

	waitingEvents map[hotstuff.Instance]map[reflect.Type][]any

	handlers map[hotstuff.Instance]map[reflect.Type][]handler

	tickers       map[int]*ticker
	tickerID      int
	instanceCount int // number of consensus instances
}

// NewPiped returns a new event loop with the requested buffer size.
func NewPiped(bufferSize uint, instanceCount int) *EventLoop {
	el := &EventLoop{
		ctx:           context.Background(),
		eventQ:        newQueue(bufferSize),
		waitingEvents: make(map[hotstuff.Instance]map[reflect.Type][]any),
		handlers:      make(map[hotstuff.Instance]map[reflect.Type][]handler),
		tickers:       make(map[int]*ticker),
	}

	el.instanceCount = instanceCount
	if el.instanceCount == 0 {
		el.instanceCount = 1
	}

	return el
}

// RegisterObserver registers a handler with priority.
// Deprecated: use RegisterHandler and the Prioritize option instead.
func (el *EventLoop) RegisterObserver(eventType any, handler EventHandler) int {
	return el.registerHandler(eventType, []HandlerOption{Prioritize()}, handler)
}

// UnregisterObserver unregister a handler.
// Deprecated: use UnregisterHandler instead.
func (el *EventLoop) UnregisterObserver(eventType any, onPipe hotstuff.Instance, id int) {
	el.UnregisterHandler(eventType, onPipe, id)
}

// RegisterHandler registers the given event handler for the given event type with the given handler options, if any.
// If no handler options are provided, the default handler options will be used.
func (el *EventLoop) RegisterHandler(eventType any, handler EventHandler, opts ...HandlerOption) int {
	return el.registerHandler(eventType, opts, handler)
}

func (el *EventLoop) registerHandler(eventType any, opts []HandlerOption, callback EventHandler) int {
	h := handler{callback: callback}

	for _, opt := range opts {
		opt(&h.opts)
	}

	el.mut.Lock()
	defer el.mut.Unlock()
	t := reflect.TypeOf(eventType)

	_, ok := el.handlers[h.opts.consensusInstance]
	if !ok {
		el.handlers[h.opts.consensusInstance] = make(map[reflect.Type][]handler)
	}

	handlers := el.handlers[h.opts.consensusInstance][t]

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

	el.handlers[h.opts.consensusInstance][t] = handlers

	return i
}

// UnregisterHandler unregisters the handler for the given event type with the given id.
func (el *EventLoop) UnregisterHandler(eventType any, onPipe hotstuff.Instance, id int) {
	el.mut.Lock()
	defer el.mut.Unlock()
	t := reflect.TypeOf(eventType)
	el.handlers[onPipe][t][id].callback = nil
}

// AddEvent adds an event to the event queue.
func (el *EventLoop) AddEvent(event any) {
	if event != nil {
		// run handlers with runInAddEvent option
		el.processEvent(event, hotstuff.ZeroInstance, true)
		el.eventQ.push(pipedEventWrapper{
			event:             event,
			consensusInstance: hotstuff.ZeroInstance,
		})
	}
}

// DebugEvent adds an event to the event queue if debug mode is enabled.
// Does nothing otherwise
func (el *EventLoop) DebugEvent(event any) {
	// TODO: Check for a debug flag
	el.AddEvent(event)
}

// PipeEvent adds an event to a specified hotstuff.
func (el *EventLoop) PipeEvent(instance hotstuff.Instance, event any) {
	if !hotstuff.IsPipelined(instance) {
		el.AddEvent(event)
		return
	}

	if event != nil {
		// run handlers with runInAddEvent option
		el.processEvent(event, instance, true)
		el.eventQ.push(pipedEventWrapper{
			event:             event,
			consensusInstance: instance,
		})
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
		wrapper := event.(pipedEventWrapper)
		if e, ok := wrapper.event.(startTickerEvent); ok {
			el.startTicker(e.tickerID)
			continue
		}
		el.processEvent(wrapper.event, wrapper.consensusInstance, false)
	}

	// HACK: when we get canceled, we will handle the events that were in the queue at that time before quitting.
	l := el.eventQ.len()
	for i := 0; i < l; i++ {
		event, _ := el.eventQ.pop()
		wrapper := event.(pipedEventWrapper)
		el.processEvent(wrapper.event, wrapper.consensusInstance, false)
	}
}

// Tick processes a single event. Returns true if an event was handled.
func (el *EventLoop) Tick(ctx context.Context) bool {
	el.setContext(ctx)

	event, ok := el.eventQ.pop()
	if !ok {
		return false
	}
	wrapper := event.(pipedEventWrapper)
	if e, ok := wrapper.event.(startTickerEvent); ok {
		el.startTicker(e.tickerID)
	} else {
		el.processEvent(wrapper.event, wrapper.consensusInstance, false)
	}

	return true
}

var handlerListPool = gpool.New(func() []handler { return make([]handler, 0, 10) })

// processEvent dispatches the event to the correct handler.
func (el *EventLoop) processEvent(event any, onPipe hotstuff.Instance, runningInAddEvent bool) {
	t := reflect.TypeOf(event)

	if !runningInAddEvent {
		defer el.dispatchDelayedEvents(onPipe, t)
	}

	// Must copy handlers to a list so that they can be executed after unlocking the mutex.
	// This looks like it might be slow, but there should be few handlers (< 10) registered for each event type.
	// We use a pool to reduce memory allocations.
	priorityList := handlerListPool.Get()
	handlerList := handlerListPool.Get()

	el.mut.Lock()
	handlers := el.handlers[onPipe][t]
	for _, handler := range handlers {
		if handler.opts.runInAddEvent != runningInAddEvent || handler.callback == nil {
			continue
		}

		if handler.opts.priority {
			priorityList = append(priorityList, handler)
		} else {
			handlerList = append(handlerList, handler)
		}
	}
	el.mut.Unlock()

	for _, handler := range priorityList {
		handler.callback(event)
	}

	priorityList = priorityList[:0]
	handlerListPool.Put(priorityList)

	for _, handler := range handlerList {
		handler.callback(event)
	}

	handlerList = handlerList[:0]
	handlerListPool.Put(handlerList)
}

func (el *EventLoop) dispatchDelayedEvents(instance hotstuff.Instance, t reflect.Type) {
	var (
		events []any
		ok     bool
	)

	if _, ok := el.waitingEvents[instance]; !ok {
		return
	}

	el.mut.Lock()
	if events, ok = el.waitingEvents[instance][t]; ok {
		delete(el.waitingEvents[instance], t)
	}
	el.mut.Unlock()

	for _, event := range events {
		el.PipeEvent(instance, event)
	}
}

func (el *EventLoop) DelayPiped(instance hotstuff.Instance, eventType, event any) {
	if eventType == nil || event == nil {
		return
	}

	if _, ok := el.waitingEvents[instance]; !ok {
		el.waitingEvents[instance] = make(map[reflect.Type][]any)
	}

	el.mut.Lock()
	t := reflect.TypeOf(eventType)
	v := el.waitingEvents[instance][t]
	v = append(v, event)
	el.waitingEvents[instance][t] = v
	el.mut.Unlock()
}

// DelayUntil allows us to delay handling of an event until after another event has happened.
// The eventType parameter decides the type of event to wait for, and it should be the zero value
// of that event type. The event parameter is the event that will be delayed.
func (el *EventLoop) DelayUntil(eventType, event any) {
	el.DelayPiped(hotstuff.ZeroInstance, eventType, event)
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
	el.eventQ.push(pipedEventWrapper{
		event:             startTickerEvent{id},
		consensusInstance: hotstuff.ZeroInstance,
	})

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

func (el *EventLoop) startTicker(id int) {
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
