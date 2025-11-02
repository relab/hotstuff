package synchronizer

import (
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/security/cert"
)

// Simple implements a simple timeout rule.
type Simple struct {
	config *core.RuntimeConfig
	auth   *cert.Authority
}

// NewSimple returns a simple timeout rule instance.
func NewSimple(
	config *core.RuntimeConfig,
	auth *cert.Authority,
) *Simple {
	return &Simple{
		config: config,
		auth:   auth,
	}
}

func (s *Simple) LocalTimeoutRule(view hotstuff.View, syncInfo hotstuff.SyncInfo) (*hotstuff.TimeoutMsg, error) {
	sig, err := s.auth.Sign(view.ToBytes())
	if err != nil {
		return nil, fmt.Errorf("failed to sign view %d: %w", view, err)
	}
	return &hotstuff.TimeoutMsg{
		ID:            s.config.ID(),
		View:          view,
		SyncInfo:      syncInfo,
		ViewSignature: sig,
	}, nil
}

func (s *Simple) RemoteTimeoutRule(_, timeoutView hotstuff.View, timeouts []hotstuff.TimeoutMsg) (hotstuff.SyncInfo, error) {
	tc, err := s.auth.CreateTimeoutCert(timeoutView, timeouts)
	if err != nil {
		return hotstuff.SyncInfo{}, fmt.Errorf("failed to create timeout certificate: %w", err)
	}
	return hotstuff.NewSyncInfoWith(tc), nil
}

func (s *Simple) VerifySyncInfo(syncInfo hotstuff.SyncInfo) (qc *hotstuff.QuorumCert, view hotstuff.View, timeout bool, err error) {
	if timeoutCert, haveTC := syncInfo.TC(); haveTC {
		if err := s.auth.VerifyTimeoutCert(timeoutCert); err != nil {
			return nil, 0, timeout, fmt.Errorf("failed to verify timeout certificate: %w", err)
		}
		view = timeoutCert.View()
		timeout = true
	}

	if quorumCert, haveQC := syncInfo.QC(); haveQC {
		if err := s.auth.VerifyQuorumCert(quorumCert); err != nil {
			return nil, 0, timeout, fmt.Errorf("failed to verify quorum certificate: %w", err)
		}
		// if there is both a TC and a QC, we use the QC if its view is greater or equal to the TC.
		if quorumCert.View() >= view {
			view = quorumCert.View()
			timeout = false
		}
		return &quorumCert, view, timeout, nil
	}
	return nil, view, timeout, nil // quorum certificate not present, so no high QC available
}
