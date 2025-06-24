package synchronizer

import (
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/security/cert"
)

// TODO(meling): Decide on an appropriate module name for the aggregate rule; it could also be named "fasthotstuff" or something else.
const ModuleNameAggregate = "aggregate"

// Aggregate implements an aggregate timeout rule.
type Aggregate struct {
	config *core.RuntimeConfig
	auth   *cert.Authority
}

// NewAggregate returns an aggregate timeout rule instance.
func NewAggregate(
	config *core.RuntimeConfig,
	auth *cert.Authority,
) *Aggregate {
	return &Aggregate{
		config: config,
		auth:   auth,
	}
}

func (s *Aggregate) LocalTimeoutRule(view hotstuff.View, syncInfo hotstuff.SyncInfo) (*hotstuff.TimeoutMsg, error) {
	sig, err := s.auth.Sign(view.ToBytes())
	if err != nil {
		return nil, fmt.Errorf("failed to sign view %d: %w", view, err)
	}
	timeoutMsg := &hotstuff.TimeoutMsg{
		ID:            s.config.ID(),
		View:          view,
		SyncInfo:      syncInfo,
		ViewSignature: sig,
	}

	// generate a second signature that will become part of the aggregateQC
	sig, err = s.auth.Sign(timeoutMsg.ToBytes())
	if err != nil {
		return nil, fmt.Errorf("failed to sign timeout message: %w", err)
	}
	timeoutMsg.MsgSignature = sig

	return timeoutMsg, nil
}

func (s *Aggregate) RemoteTimeoutRule(currentView, timeoutView hotstuff.View, timeouts []hotstuff.TimeoutMsg) (hotstuff.SyncInfo, error) {
	tc, err := s.auth.CreateTimeoutCert(timeoutView, timeouts)
	if err != nil {
		return hotstuff.SyncInfo{}, fmt.Errorf("failed to create timeout certificate: %w", err)
	}
	aggQC, err := s.auth.CreateAggregateQC(currentView, timeouts)
	if err != nil {
		return hotstuff.SyncInfo{}, fmt.Errorf("failed to create aggregate quorum certificate: %w", err)
	}
	return hotstuff.NewSyncInfo().WithTC(tc).WithAggQC(aggQC), nil
}
