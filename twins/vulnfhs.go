package twins

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/rules"
	"github.com/relab/hotstuff/security/blockchain"
)

const nameVulnerableFHS = "vulnerableFHS"

// A wrapper around the FHS rules that swaps the commit rule for a vulnerable version
type vulnerableFHS struct {
	logger     logging.Logger
	blockchain *blockchain.Blockchain
	rules.FastHotStuff
}

func NewVulnFHS(
	logger logging.Logger,
	blockchain *blockchain.Blockchain,
	inner *rules.FastHotStuff,
) *vulnerableFHS {
	return &vulnerableFHS{
		logger:       logger,
		blockchain:   blockchain,
		FastHotStuff: *inner,
	}
}

func (fhs *vulnerableFHS) qcRef(qc hotstuff.QuorumCert) (*hotstuff.Block, bool) {
	if (hotstuff.Hash{}) == qc.BlockHash() {
		return nil, false
	}
	return fhs.blockchain.Get(qc.BlockHash())
}

// CommitRule decides whether an ancestor of the block can be committed.
func (fhs *vulnerableFHS) CommitRule(block *hotstuff.Block) *hotstuff.Block {
	parent, ok := fhs.qcRef(block.QuorumCert())
	if !ok {
		return nil
	}
	fhs.logger.Debug("PRECOMMIT: ", parent)
	grandparent, ok := fhs.qcRef(parent.QuorumCert())
	if !ok {
		return nil
	}
	// NOTE: this does check for a direct link between the block and the grandparent.
	// This is what causes the safety violation.
	if block.Parent() == parent.Hash() && parent.Parent() == grandparent.Hash() {
		fhs.logger.Debug("COMMIT(vulnerable): ", grandparent)
		return grandparent
	}
	return nil
}

var _ consensus.Ruleset = (*vulnerableFHS)(nil)
