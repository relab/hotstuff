package twins

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/protocol"
)

type timeoutManager struct {
	eventLoop  *eventloop.EventLoop
	viewStates *protocol.ViewStates

	node      *node
	network   *Network
	countdown int
	timeout   int
}

func (tm *timeoutManager) advance() {
	tm.countdown--
	if tm.countdown == 0 {
		view := tm.viewStates.View()
		tm.eventLoop.AddEvent(hotstuff.TimeoutEvent{View: view})
		tm.countdown = tm.timeout
		if tm.node.effectiveView <= view {
			tm.node.effectiveView = view + 1
			tm.network.logger.Infof("node %v effective view is %d due to timeout", tm.node.id, tm.node.effectiveView)
		}
	}
}

func (tm *timeoutManager) viewChange(event hotstuff.ViewChangeEvent) {
	tm.countdown = tm.timeout
	if event.Timeout {
		tm.network.logger.Infof("node %v entered view %d after timeout", tm.node.id, event.View)
	} else {
		tm.network.logger.Infof("node %v entered view %d after voting", tm.node.id, event.View)
	}
}

func newTimeoutManager(
	network *Network,
	node *node,
	eventLoop *eventloop.EventLoop,
	viewStates *protocol.ViewStates,
) *timeoutManager {
	tm := &timeoutManager{
		node:       node,
		network:    network,
		eventLoop:  eventLoop,
		viewStates: viewStates,
		timeout:    5,
	}
	tm.eventLoop.RegisterHandler(tick{}, func(_ any) {
		tm.advance()
	}, eventloop.Prioritize())
	tm.eventLoop.RegisterHandler(hotstuff.ViewChangeEvent{}, func(event any) {
		tm.viewChange(event.(hotstuff.ViewChangeEvent))
	}, eventloop.Prioritize())
	return tm
}
