// Package chainedhotstuff implements the pipelined three-chain version of the HotStuff protocol.
package chainedhotstuff

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/modules"
)

func init() {
	modules.RegisterModule("chainedhotstuff", New)
}

// ChainedHotStuff implements the pipelined three-phase HotStuff protocol.
type ChainedHotStuff struct {
	mods *modules.ConsensusCore

	// protocol variables

	bLock *hotstuff.Block // the currently locked block
}

// New returns a new chainedhotstuff instance.
func New() consensus.Rules {
	return &ChainedHotStuff{
		bLock: hotstuff.GetGenesis(),
	}
}

// InitModule gives the module a reference to the ConsensusCore object.
// It also allows the module to set module options using the OptionsBuilder.
func (hs *ChainedHotStuff) InitModule(mods *modules.ConsensusCore, _ *modules.OptionsBuilder) {
	hs.mods = mods
}

func (hs *ChainedHotStuff) qcRef(qc hotstuff.QuorumCert) (*hotstuff.Block, bool) {
	if (hotstuff.Hash{}) == qc.BlockHash() {
		return nil, false
	}
	return hs.mods.BlockChain().Get(qc.BlockHash())
}

// CommitRule decides whether an ancestor of the block should be committed.
func (hs *ChainedHotStuff) CommitRule(block *hotstuff.Block) *hotstuff.Block {
	block1, ok := hs.qcRef(block.QuorumCert())
	if !ok {
		return nil
	}

	// Note that we do not call UpdateHighQC here.
	// This is done through AdvanceView, which the Consensus implementation will call.
	hs.mods.Logger().Debug("PRE_COMMIT: ", block1)

	block2, ok := hs.qcRef(block1.QuorumCert())
	if !ok {
		return nil
	}

	if block2.View() > hs.bLock.View() {
		hs.mods.Logger().Debug("COMMIT: ", block2)
		hs.bLock = block2
	}

	block3, ok := hs.qcRef(block2.QuorumCert())
	if !ok {
		return nil
	}

	if block1.Parent() == block2.Hash() && block2.Parent() == block3.Hash() {
		hs.mods.Logger().Debug("DECIDE: ", block3)
		return block3
	}

	return nil
}

// VoteRule decides whether to vote for the proposal or not.
func (hs *ChainedHotStuff) VoteRule(proposal hotstuff.ProposeMsg) bool {
	block := proposal.Block

	qcBlock, haveQCBlock := hs.mods.BlockChain().Get(block.QuorumCert().BlockHash())

	safe := false
	if haveQCBlock && qcBlock.View() > hs.bLock.View() {
		safe = true
	} else {
		hs.mods.Logger().Debug("OnPropose: liveness condition failed")
		// check if this block extends bLock
		if hs.mods.BlockChain().Extends(block, hs.bLock) {
			safe = true
		} else {
			hs.mods.Logger().Debug("OnPropose: safety condition failed")
		}
	}

	return safe
}

// ChainLength returns the number of blocks that need to be chained together in order to commit.
func (hs *ChainedHotStuff) ChainLength() int {
	return 3
}
