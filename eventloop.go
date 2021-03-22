package hotstuff

import "context"

// EventLoop synchronously executes a queue of events.
type EventLoop struct {
	mod    *HotStuff
	eventQ chan Event
}

// NewEventLoop returns a new event loop with the requested buffer size.
func NewEventLoop(bufferSize uint) *EventLoop {
	return &EventLoop{
		eventQ: make(chan Event, bufferSize),
	}
}

// InitModule gives the module a reference to the HotStuff object.
func (el *EventLoop) InitModule(hs *HotStuff) {
	el.mod = hs
}

// AddEvent adds an event to the event queue. The event may be processed before it enters the queue.
func (el *EventLoop) AddEvent(event Event) {
	// TODO: consider making it possible to register as an event handler for a specific kind of event.
	// We could also have two types of handlers that run at different times.
	// For example, the blockchain should process events at the time that they arrive.
	// On the other hand, the consensus algorithm must wait until the event is at the front of the queue.
	// It should also be possible to "consume" an event such that it is not added to the event queue,
	// for example if we were unable to verify a signature.

	// We let the blockchain process the event first, if it is able to, so that it may store blocks early.
	// This could help avoid unnecessarily making fetch requests when blocks arrive out of order.
	if ep, ok := el.mod.BlockChain().(EventProcessor); ok {
		ep.ProcessEvent(event)
	}
	el.eventQ <- event
}

// Run runs the event loop. A context object can be provided to stop the event loop.
func (el *EventLoop) Run(ctx context.Context) {
	// We start the view synchronizer from this goroutine such that we can avoid data races between Propose()
	// and event handlers.
	el.mod.ViewSynchronizer().Start()

	for {
		select {
		case event := <-el.eventQ:
			el.processEvent(event)
		case <-ctx.Done():
			return
		}
	}
}

// processEvent dispatches the event to the correct handler.
func (el *EventLoop) processEvent(e Event) {
	switch event := e.(type) {
	case ProposeMsg:
		el.mod.Consensus().OnPropose(event)
	case VoteMsg:
		el.mod.Consensus().OnVote(event)
	case TimeoutMsg:
		el.mod.ViewSynchronizer().OnRemoteTimeout(event)
	case NewViewMsg:
		el.mod.ViewSynchronizer().OnNewView(event)
	}
}
