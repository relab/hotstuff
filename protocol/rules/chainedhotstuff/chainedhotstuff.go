// Package chainedhotstuff implements the pipelined three-chain version of the HotStuff protocol.
package chainedhotstuff

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/security/blockchain"
)

const ModuleName = "chainedhotstuff"

// ChainedHotStuff implements the pipelined three-phase HotStuff protocol.
type ChainedHotStuff struct {
	logger     logging.Logger
	blockChain *blockchain.BlockChain

	// protocol variables

	bLock *hotstuff.Block // the currently locked block
}

// New returns a new chainedhotstuff instance.
func New(
	logger logging.Logger,
	blockChain *blockchain.BlockChain,
) modules.HotstuffRuleset {
	return &ChainedHotStuff{
		blockChain: blockChain,
		logger:     logger,

		bLock: hotstuff.GetGenesis(),
	}
}

func (hs *ChainedHotStuff) qcRef(qc hotstuff.QuorumCert) (*hotstuff.Block, bool) {
	if (hotstuff.Hash{}) == qc.BlockHash() {
		return nil, false
	}
	return hs.blockChain.Get(qc.BlockHash())
}

// CommitRule decides whether an ancestor of the block should be committed.
func (hs *ChainedHotStuff) CommitRule(block *hotstuff.Block) *hotstuff.Block {
	block1, ok := hs.qcRef(block.QuorumCert())
	if !ok {
		return nil
	}

	// Note that we do not call UpdateHighQC here.
	// This is done through AdvanceView, which the Consensus implementation will call.
	hs.logger.Debug("CommitRule - PRE_COMMIT: ", block1)

	block2, ok := hs.qcRef(block1.QuorumCert())
	if !ok {
		return nil
	}

	if block2.View() > hs.bLock.View() {
		hs.logger.Debug("CommitRule - COMMIT: ", block2)
		hs.bLock = block2
	}

	block3, ok := hs.qcRef(block2.QuorumCert())
	if !ok {
		return nil
	}

	if block1.Parent() == block2.Hash() && block2.Parent() == block3.Hash() {
		hs.logger.Debug("CommitRule - DECIDE: ", block3)
		return block3
	}

	return nil
}

// VoteRule decides whether to vote for the proposal or not.
func (hs *ChainedHotStuff) VoteRule(_ hotstuff.View, proposal hotstuff.ProposeMsg) bool {
	block := proposal.Block

	qcBlock, haveQCBlock := hs.blockChain.Get(block.QuorumCert().BlockHash())

	safe := false
	if haveQCBlock && qcBlock.View() > hs.bLock.View() {
		safe = true
	} else {
		hs.logger.Debug("VoteRule: liveness condition failed")
		// check if this block extends bLock
		if hs.blockChain.Extends(block, hs.bLock) {
			safe = true
		} else {
			hs.logger.Debug("VoteRule: safety condition failed")
		}
	}

	return safe
}

// ChainLength returns the number of blocks that need to be chained together in order to commit.
func (hs *ChainedHotStuff) ChainLength() int {
	return 3
}
