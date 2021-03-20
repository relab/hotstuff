package hotstuff

type EventLoop struct {
	mod    *HotStuff
	eventQ chan Event
}

func NewEventLoop(bufferSize uint) *EventLoop {
	return &EventLoop{
		eventQ: make(chan Event, bufferSize),
	}
}

// InitModule gives the module a reference to the HotStuff object.
func (el *EventLoop) InitModule(hs *HotStuff) {
	el.mod = hs
}

func (el *EventLoop) AddEvent(event Event) {
	// We let the blockchain process the event first, if it is able to, so that it may store blocks early.
	// This could help avoid unnecessarily making fetch requests when blocks arrive out of order.
	if ep, ok := el.mod.BlockChain().(EventProcessor); ok {
		ep.ProcessEvent(event)
	}
	el.eventQ <- event
}

func (el *EventLoop) Run() {
	for event := range el.eventQ {
		el.processEvent(event)
	}
}

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
