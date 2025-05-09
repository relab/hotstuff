package consensus

import (
	"github.com/relab/hotstuff/internal/tree"
	"github.com/relab/hotstuff/protocol/kauri"
)

type Option func(*Consensus)

func WithKauri(tree *tree.Tree) Option {
	return func(cs *Consensus) {
		cs.kauri = kauri.New(
			cs.auth,
			cs.leaderRotation,
			cs.blockChain,
			cs.opts,
			cs.eventLoop,
			cs.configuration,
			cs.sender,
			cs.logger,
			tree,
		)
	}
}
