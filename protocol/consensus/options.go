package consensus

import (
	"github.com/relab/hotstuff/internal/tree"
	"github.com/relab/hotstuff/protocol/kauri"
)

type Option func(*Consensus)

// TODO(AlanRostem): consider removing this since it's usage is moved to globals.Globals
func WithKauri(tree *tree.Tree) Option {
	return func(cs *Consensus) {
		cs.kauri = kauri.New(
			cs.auth,
			cs.leaderRotation,
			cs.blockChain,
			cs.globals,
			cs.eventLoop,
			cs.sender,
			cs.logger,
			tree,
		)
	}
}
