package twins

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/protocol"
	"go.uber.org/zap"
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
			tm.network.logger.Info("node effective view updated due to timeout", zap.Stringer("node", tm.node.id), zap.Uint64("view", uint64(tm.node.effectiveView)))
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
	el *eventloop.EventLoop,
	viewStates *protocol.ViewStates,
) *timeoutManager {
	tm := &timeoutManager{
		node:       node,
		network:    network,
		eventLoop:  el,
		viewStates: viewStates,
		timeout:    5,
	}
	eventloop.Register(el, func(_ tick) {
		tm.advance()
	}, eventloop.Prioritize())
	eventloop.Register(el, func(event hotstuff.ViewChangeEvent) {
		tm.viewChange(event)
	}, eventloop.Prioritize())
	return tm
}
