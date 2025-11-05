package rules

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/security/blockchain"
)

const NameChainedHotStuff = "chainedhotstuff"

// ChainedHotStuff implements the pipelined three-phase HotStuff protocol.
type ChainedHotStuff struct {
	logger     logging.Logger
	config     *core.RuntimeConfig
	blockchain *blockchain.Blockchain

	// protocol variables

	bLock *hotstuff.Block // the currently locked block
}

// NewChainedHotStuff returns a new instance of the chained HotStuff consensus ruleset.
func NewChainedHotStuff(
	logger logging.Logger,
	config *core.RuntimeConfig,
	blockchain *blockchain.Blockchain,
) *ChainedHotStuff {
	return &ChainedHotStuff{
		logger:     logger,
		config:     config,
		blockchain: blockchain,

		bLock: hotstuff.GetGenesis(),
	}
}

func (hs *ChainedHotStuff) qcRef(qc hotstuff.QuorumCert) (*hotstuff.Block, bool) {
	if (hotstuff.Hash{}) == qc.BlockHash() {
		return nil, false
	}
	return hs.blockchain.Get(qc.BlockHash())
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

	// since our implementation does not create dummy blocks every view, 
	// we explicitly check that the parents are in the previous view
	if block1.Parent() == block2.Hash() && 
		block2.Parent() == block3.Hash() &&
		block1.View() == block3.View()+2{
		hs.logger.Debug("CommitRule - DECIDE: ", block3)
		return block3
	}

	return nil
}

// VoteRule decides whether to vote for the proposal or not.
func (hs *ChainedHotStuff) VoteRule(_ hotstuff.View, proposal hotstuff.ProposeMsg) bool {
	block := proposal.Block

	hash := block.QuorumCert().BlockHash()
	qcBlock, haveQCBlock := hs.blockchain.Get(hash)

	safe := false
	if haveQCBlock && qcBlock.View() > hs.bLock.View() {
		safe = true
	} else {
		hs.logger.Debug("VoteRule: liveness condition failed")
		// check if this block extends bLock
		if hs.blockchain.Extends(block, hs.bLock) {
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

// ProposeRule returns a new hotstuff proposal based on the current view, quorum certificate, and command batch.
func (hs *ChainedHotStuff) ProposeRule(view hotstuff.View, cert hotstuff.SyncInfo, cmd *clientpb.Batch) (proposal hotstuff.ProposeMsg, ok bool) {
	qc, ok := cert.QC()
	if !ok {
		return proposal, false
	}
	proposal = hotstuff.NewProposeMsg(hs.config.ID(), view, qc, cmd)
	return proposal, true
}

var _ consensus.Ruleset = (*ChainedHotStuff)(nil)
