package rules

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/security/blockchain"
)

const NameFastHotStuff = "fasthotstuff"

// FastHotStuff is an implementation of the Fast-HotStuff protocol.
// See the paper for details: https://arxiv.org/abs/2010.11454
type FastHotStuff struct {
	logger     logging.Logger
	config     *core.RuntimeConfig
	blockchain *blockchain.Blockchain
}

// NewFastHotStuff returns a new instance of the FastHotStuff consensus ruleset.
func NewFastHotStuff(
	logger logging.Logger,
	config *core.RuntimeConfig,
	blockchain *blockchain.Blockchain,
) *FastHotStuff {
	if !config.HasAggregateQC() {
		panic(NameFastHotStuff + " requires aggregated quorum certificates")
	}
	return &FastHotStuff{
		logger:     logger,
		config:     config,
		blockchain: blockchain,
	}
}

func (fhs *FastHotStuff) qcRef(qc hotstuff.QuorumCert) (*hotstuff.Block, bool) {
	if (hotstuff.Hash{}) == qc.BlockHash() {
		return nil, false
	}
	return fhs.blockchain.Get(qc.BlockHash())
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
func (fhs *FastHotStuff) VoteRule(view hotstuff.View, proposal hotstuff.ProposeMsg) bool {
	// The base implementation verifies both regular QCs and AggregateQCs, and asserts that the QC embedded in the
	// block is the same as the highQC found in the aggregateQC.
	if proposal.AggregateQC != nil {
		hqcBlock, ok := fhs.blockchain.Get(proposal.Block.QuorumCert().BlockHash())
		return ok && fhs.blockchain.Extends(proposal.Block, hqcBlock)
	}
	return proposal.Block.View() >= view &&
		proposal.Block.View() == proposal.Block.QuorumCert().View()+1
}

// ChainLength returns the number of blocks that need to be chained together in order to commit.
func (fhs *FastHotStuff) ChainLength() int {
	return 2
}

// ProposeRule returns a new fast hotstuff proposal based on the current view, (aggregate) quorum certificate, and command batch.
func (fhs *FastHotStuff) ProposeRule(view hotstuff.View, cert hotstuff.SyncInfo, cmd *clientpb.Batch) (proposal hotstuff.ProposeMsg, ok bool) {
	qc, ok := cert.QC()
	if !ok {
		return proposal, false
	}
	proposal = hotstuff.NewProposeMsg(fhs.config.ID(), view, qc, cmd)
	if aggQC, ok := cert.AggQC(); ok {
		proposal.AggregateQC = &aggQC
	}
	return proposal, true
}

var _ consensus.Ruleset = (*FastHotStuff)(nil)
