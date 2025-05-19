package consensus

import (
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol/kauri"
)

type Option func(*Consensus)

// TODO(AlanRostem): consider removing this since it's usage is moved to core.RuntimeConfig
func WithKauri() Option {
	return func(cs *Consensus) {
		cs.kauri = kauri.New(
			cs.logger,
			cs.eventLoop,
			cs.config,
			cs.blockChain,
			cs.auth,
			cs.sender,
		)
	}
}

func OverrideProposeRule(impl modules.ProposeRuler) Option {
	return func(c *Consensus) {
		c.ruler = impl
	}
}
