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

// newSimple returns a simple timeout rule instance.
func newSimple(
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

var _ TimeoutRuler = (*Simple)(nil)
