package synchronizer

import (
	"slices"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
)

type timeoutCollector struct {
	config   *core.RuntimeConfig
	timeouts []hotstuff.TimeoutMsg
}

func newTimeoutCollector(config *core.RuntimeConfig) *timeoutCollector {
	return &timeoutCollector{
		config: config,
	}
}

// add returns true if a quorum of timeouts has been collected for the view of given timeout message.
func (s *timeoutCollector) add(timeout hotstuff.TimeoutMsg) ([]hotstuff.TimeoutMsg, bool) {
	// needs to be done later since the config's quorum size is set after this one's init
	if s.timeouts == nil {
		s.timeouts = make([]hotstuff.TimeoutMsg, 0, 2*s.config.QuorumSize())
	}
	// ignore this timeout if we already have a timeout from this replica in this view
	if slices.ContainsFunc(s.timeouts, func(t hotstuff.TimeoutMsg) bool {
		return t.View == timeout.View && t.ID == timeout.ID
	}) {
		return nil, false
	}
	s.timeouts = append(s.timeouts, timeout)
	if len(s.timeouts) < s.config.QuorumSize() {
		return nil, false
	}
	timeoutList := slices.Clone(s.timeouts)
	// remove timeouts for this view from the slice, since we now have a quorum
	// and we don't need to keep them around anymore.
	s.timeouts = slices.DeleteFunc(s.timeouts, func(t hotstuff.TimeoutMsg) bool { return t.View == timeout.View })
	return timeoutList, true
}

// deleteOldViews removes all timeouts with a view lower than the current view.
// This is used to clean up timeouts that are no longer relevant, as they are from
// an already processed view.
func (s *timeoutCollector) deleteOldViews(currentView hotstuff.View) {
	s.timeouts = slices.DeleteFunc(s.timeouts, func(t hotstuff.TimeoutMsg) bool { return t.View < currentView })
}
