// Package fasthotstuff implements the two-chain Fast-HotStuff protocol.
package fasthotstuff

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
)

func init() {
	modules.RegisterModule("fasthotstuff", New)
}

// FastHotStuff is an implementation of the Fast-HotStuff protocol.
type FastHotStuff struct {
	blockChain   modules.BlockChain
	logger       logging.Logger
	synchronizer modules.Synchronizer
}

// New returns a new FastHotStuff instance.
func New() consensus.Rules {
	return &FastHotStuff{}
}

// InitModule initializes the module.
func (fhs *FastHotStuff) InitModule(mods *modules.Core) {
	var opts *modules.Options

	mods.Get(&opts, &fhs.blockChain, &fhs.logger, &fhs.synchronizer)

	opts.SetShouldUseAggQC()
}

func (fhs *FastHotStuff) qcRef(qc hotstuff.QuorumCert) (*hotstuff.Block, bool) {
	if (hotstuff.Hash{}) == qc.BlockHash() {
		return nil, false
	}
	return fhs.blockChain.Get(qc.BlockHash())
}

// CommitRule decides whether an ancestor of the block can be committed.
func (fhs *FastHotStuff) CommitRule(block *hotstuff.Block) *hotstuff.Block {
	parent, ok := fhs.qcRef(block.QuorumCert())
	if !ok {
		return nil
	}
	fhs.logger.Debug("PRECOMMIT: ", parent)
	grandparent, ok := fhs.qcRef(parent.QuorumCert())
	if !ok {
		return nil
	}
	if block.Parent() == parent.Hash() && block.View() == parent.View()+1 &&
		parent.Parent() == grandparent.Hash() && parent.View() == grandparent.View()+1 {
		fhs.logger.Debug("COMMIT: ", grandparent)
		return grandparent
	}
	return nil
}

// VoteRule decides whether to vote for the proposal or not.
func (fhs *FastHotStuff) VoteRule(proposal hotstuff.ProposeMsg) bool {
	// The base implementation verifies both regular QCs and AggregateQCs, and asserts that the QC embedded in the
	// block is the same as the highQC found in the aggregateQC.
	if proposal.AggregateQC != nil {
		hqcBlock, ok := fhs.blockChain.Get(proposal.Block.QuorumCert().BlockHash())
		return ok && fhs.blockChain.Extends(proposal.Block, hqcBlock)
	}
	return proposal.Block.View() >= fhs.synchronizer.View() &&
		proposal.Block.View() == proposal.Block.QuorumCert().View()+1
}

// ChainLength returns the number of blocks that need to be chained together in order to commit.
func (fhs *FastHotStuff) ChainLength() int {
	return 2
}
