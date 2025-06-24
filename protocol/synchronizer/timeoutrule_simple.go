package synchronizer

import (
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/security/cert"
)

const ModuleNameSimple = "simple"

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
