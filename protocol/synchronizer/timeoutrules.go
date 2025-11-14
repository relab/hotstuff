package synchronizer

import "github.com/relab/hotstuff"

type TimeoutRuler interface {
	LocalTimeoutRule(hotstuff.View, hotstuff.SyncInfo) (*hotstuff.TimeoutMsg, error)
	RemoteTimeoutRule(currentView, timeoutView hotstuff.View, timeouts []hotstuff.TimeoutMsg) (hotstuff.SyncInfo, error)
}
