package synchronizer

import (
	"slices"

	"github.com/relab/hotstuff"
)

type timeoutCollector struct {
	timeouts   []hotstuff.TimeoutMsg
	quorumSize int
}

func newTimeoutCollector(quorumSize int) *timeoutCollector {
	return &timeoutCollector{
		timeouts:   make([]hotstuff.TimeoutMsg, 0, 2*quorumSize),
		quorumSize: quorumSize,
	}
}

// add returns true if a quorum of timeouts has been collected for the view of given timeout message.
func (s *timeoutCollector) add(timeout hotstuff.TimeoutMsg) ([]hotstuff.TimeoutMsg, bool) {
	// ignore this timeout if we already have a timeout from this replica in this view
	if slices.ContainsFunc(s.timeouts, func(t hotstuff.TimeoutMsg) bool {
		return t.View == timeout.View && t.ID == timeout.ID
	}) {
		return nil, false
	}
	s.timeouts = append(s.timeouts, timeout)

	if len(s.timeouts) < s.quorumSize {
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
