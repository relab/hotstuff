// Package fasthotstuff implements the two-chain Fast-HotStuff protocol.
package fasthotstuff

import (
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/modules"
)

func init() {
	modules.RegisterModule("fasthotstuff", New)
}

// FastHotStuff is an implementation of the Fast-HotStuff protocol.
type FastHotStuff struct {
	mods *consensus.Modules
}

// New returns a new FastHotStuff instance.
func New() consensus.Rules {
	return &FastHotStuff{}
}

// InitConsensusModule gives the module a reference to the Modules object.
// It also allows the module to set module options using the OptionsBuilder.
func (fhs *FastHotStuff) InitConsensusModule(mods *consensus.Modules, opts *consensus.OptionsBuilder) {
	fhs.mods = mods
	opts.SetShouldUseAggQC()
}

func (fhs *FastHotStuff) qcRef(qc consensus.QuorumCert) (*consensus.Block, bool) {
	if (consensus.Hash{}) == qc.BlockHash() {
		return nil, false
	}
	return fhs.mods.BlockChain().Get(qc.BlockHash())
}

// CommitRule decides whether an ancestor of the block can be committed.
func (fhs *FastHotStuff) CommitRule(block *consensus.Block) *consensus.Block {
	parent, ok := fhs.qcRef(block.QuorumCert())
	if !ok {
		return nil
	}
	fhs.mods.Logger().Debug("PRECOMMIT: ", parent)
	grandparent, ok := fhs.qcRef(parent.QuorumCert())
	if !ok {
		return nil
	}
	if block.Parent() == parent.Hash() && block.View() == parent.View()+1 &&
		parent.Parent() == grandparent.Hash() && parent.View() == grandparent.View()+1 {
		fhs.mods.Logger().Debug("COMMIT: ", grandparent)
		return grandparent
	}
	return nil
}

// VoteRule decides whether to vote for the proposal or not.
func (fhs *FastHotStuff) VoteRule(proposal consensus.ProposeMsg) bool {
	// The base implementation verifies both regular QCs and AggregateQCs, and asserts that the QC embedded in the
	// block is the same as the highQC found in the aggregateQC.
	if proposal.AggregateQC != nil {
		hqcBlock, ok := fhs.mods.BlockChain().Get(proposal.Block.QuorumCert().BlockHash())
		return ok && fhs.mods.BlockChain().Extends(proposal.Block, hqcBlock)
	}
	return proposal.Block.View() >= fhs.mods.Synchronizer().View() &&
		proposal.Block.View() == proposal.Block.QuorumCert().View()+1
}

// ChainLength returns the number of blocks that need to be chained together in order to commit.
func (fhs *FastHotStuff) ChainLength() int {
	return 2
}
