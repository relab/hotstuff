package synchronizer

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/security/cert"
)

// NewTimeoutRuler returns a TimeoutRuler based on the configuration.
func NewTimeoutRuler(cfg *core.RuntimeConfig, auth *cert.Authority) TimeoutRuler {
	if cfg.HasAggregateQC() {
		return NewAggregate(cfg, auth)
	}
	return NewSimple(cfg, auth)
}

type TimeoutRuler interface {
	LocalTimeoutRule(hotstuff.View, hotstuff.SyncInfo) (*hotstuff.TimeoutMsg, error)
	RemoteTimeoutRule(currentView, timeoutView hotstuff.View, timeouts []hotstuff.TimeoutMsg) (hotstuff.SyncInfo, error)
	VerifySyncInfo(hotstuff.SyncInfo) (qc *hotstuff.QuorumCert, view hotstuff.View, timeout bool, err error)
}
