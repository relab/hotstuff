package synchronizer

import (
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/security/cert"
)

// Aggregate implements an aggregate timeout rule.
type Aggregate struct {
	config *core.RuntimeConfig
	auth   *cert.Authority
}

// newAggregate returns an aggregate timeout rule instance.
func newAggregate(
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
	si := hotstuff.NewSyncInfoWith(tc)
	si.SetAggQC(aggQC)
	return si, nil
}

func (s *Aggregate) VerifySyncInfo(syncInfo hotstuff.SyncInfo) (qc *hotstuff.QuorumCert, view hotstuff.View, timeout bool, err error) {
	if timeoutCert, haveTC := syncInfo.TC(); haveTC {
		if err := s.auth.VerifyTimeoutCert(timeoutCert); err != nil {
			return nil, 0, timeout, fmt.Errorf("failed to verify timeout certificate: %w", err)
		}
		view = timeoutCert.View()
		timeout = true
	}

	if aggQC, haveQC := syncInfo.AggQC(); haveQC {
		highQC, err := s.auth.VerifyAggregateQC(aggQC)
		if err != nil {
			return nil, 0, timeout, fmt.Errorf("failed to verify aggregate quorum certificate: %w", err)
		}
		if aggQC.View() >= view {
			view = aggQC.View()
			timeout = true
		}
		return &highQC, view, timeout, nil
	}
	return nil, view, timeout, nil // aggregate quorum certificate not present, so no high QC available
}
