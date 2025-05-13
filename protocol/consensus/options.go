package consensus

import (
	"github.com/relab/hotstuff/internal/tree"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol/kauri"
)

type Option func(*Consensus)

// TODO(AlanRostem): consider removing this since it's usage is moved to core.RuntimeConfig
func WithKauri(tree *tree.Tree) Option {
	return func(cs *Consensus) {
		cs.kauri = kauri.New(
			cs.auth,
			cs.leaderRotation,
			cs.blockChain,
			cs.config,
			cs.eventLoop,
			cs.sender,
			cs.logger,
			tree,
		)
	}
}

func OverrideProposeRule(impl modules.ProposeRuler) Option {
	return func(c *Consensus) {
		c.ruler = impl
	}
}
