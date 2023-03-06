// Package chainedhotstuff implements the pipelined three-chain version of the HotStuff protocol.
package chainedhotstuff

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/modules"
)

func init() {
	modules.RegisterModule("chainedhotstuff", New)
}

// ChainedHotStuff implements the pipelined three-phase HotStuff protocol.
type ChainedHotStuff struct {
	blockChain modules.BlockChain
	logger     logging.Logger

	// protocol variables

	bLock map[hotstuff.ChainNumber]*hotstuff.Block // the currently locked block
}

// New returns a new chainedhotstuff instance.
func New() consensus.Rules {
	bLock := make(map[hotstuff.ChainNumber]*hotstuff.Block)
	for i := 1; i <= hotstuff.NumberOfChains; i++ {
		bLock[hotstuff.ChainNumber(i)] = hotstuff.GetGenesis(hotstuff.ChainNumber(i))
	}
	return &ChainedHotStuff{
		bLock: bLock,
	}
}

// InitModule initializes the module.
func (hs *ChainedHotStuff) InitModule(mods *modules.Core) {
	mods.Get(&hs.blockChain, &hs.logger)
}

func (hs *ChainedHotStuff) qcRef(block *hotstuff.Block) (*hotstuff.Block, bool) {
	qc := block.QuorumCert()
	if (hotstuff.Hash{}) == qc.BlockHash() {
		return nil, false
	}
	return hs.blockChain.Get(block.ChainNumber(), qc.BlockHash())
}

// CommitRule decides whether an ancestor of the block should be committed.
func (hs *ChainedHotStuff) CommitRule(block *hotstuff.Block) *hotstuff.Block {
	block1, ok := hs.qcRef(block)
	if !ok {
		return nil
	}

	// Note that we do not call UpdateHighQC here.
	// This is done through AdvanceView, which the Consensus implementation will call.
	hs.logger.Debug("PRE_COMMIT: ", block1)

	block2, ok := hs.qcRef(block1)
	if !ok {
		return nil
	}

	if block2.View() > hs.bLock[block.ChainNumber()].View() {
		hs.logger.Debug("COMMIT: ", block2)
		hs.bLock[block.ChainNumber()] = block2
	}

	block3, ok := hs.qcRef(block2)
	if !ok {
		return nil
	}

	if block1.Parent() == block2.Hash() && block2.Parent() == block3.Hash() {
		hs.logger.Info("DECIDE: ", block3)
		return block3
	}

	return nil
}

// VoteRule decides whether to vote for the proposal or not.
func (hs *ChainedHotStuff) VoteRule(proposal hotstuff.ProposeMsg) bool {
	block := proposal.Block

	qcBlock, haveQCBlock := hs.blockChain.Get(block.ChainNumber(), block.QuorumCert().BlockHash())

	safe := false
	if !haveQCBlock {
		hs.logger.Debug("OnPropose:QC Block not found")
	}
	if haveQCBlock && qcBlock.View() > hs.bLock[block.ChainNumber()].View() {
		safe = true
	} else {
		hs.logger.Debug("OnPropose: liveness condition failed", qcBlock.View(), hs.bLock[block.ChainNumber()].View())
		// check if this block extends bLock
		if hs.blockChain.Extends(block, hs.bLock[block.ChainNumber()]) {
			safe = true
		} else {
			hs.logger.Debug("OnPropose: safety condition failed")
		}
	}

	return safe
}

// ChainLength returns the number of blocks that need to be chained together in order to commit.
func (hs *ChainedHotStuff) ChainLength() int {
	return 3
}
