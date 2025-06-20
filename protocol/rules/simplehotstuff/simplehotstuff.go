// Package simplehotstuff implements a simplified version of the three-chain HotStuff protocol.
package simplehotstuff

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/security/blockchain"
)

const ModuleName = "simplehotstuff"

// SimpleHotStuff implements a simplified version of the HotStuff algorithm.
//
// Based on the simplified algorithm described in the paper
// "Formal Verification of HotStuff" by Leander Jehl.
type SimpleHotStuff struct {
	logger     logging.Logger
	config     *core.RuntimeConfig
	blockchain *blockchain.Blockchain

	locked *hotstuff.Block
}

// New returns a new SimpleHotStuff instance.
func New(
	logger logging.Logger,
	config *core.RuntimeConfig,
	blockchain *blockchain.Blockchain,
) *SimpleHotStuff {
	return &SimpleHotStuff{
		logger:     logger,
		config:     config,
		blockchain: blockchain,

		locked: hotstuff.GetGenesis(),
	}
}

// VoteRule decides if the replica should vote for the given block.
func (hs *SimpleHotStuff) VoteRule(view hotstuff.View, proposal hotstuff.ProposeMsg) bool {
	block := proposal.Block

	// Rule 1: can only vote in increasing rounds
	if block.View() < view {
		hs.logger.Info("VoteRule: block view too low")
		return false
	}

	parent, ok := hs.blockchain.Get(block.QuorumCert().BlockHash())
	if !ok {
		hs.logger.Info("VoteRule: missing parent block: ", block.QuorumCert().BlockHash())
		return false
	}

	// Rule 2: can only vote if parent's view is greater than or equal to locked block's view.
	if parent.View() < hs.locked.View() {
		hs.logger.Info("OnPropose: parent too old")
		return false
	}

	return true
}

// CommitRule decides if an ancestor of the block can be committed, and returns the ancestor, otherwise returns nil.
func (hs *SimpleHotStuff) CommitRule(block *hotstuff.Block) *hotstuff.Block {
	// will consider if the great-grandparent of the new block can be committed.
	p, ok := hs.blockchain.Get(block.QuorumCert().BlockHash())
	if !ok {
		return nil
	}

	gp, ok := hs.blockchain.Get(p.QuorumCert().BlockHash())
	if ok && gp.View() > hs.locked.View() {
		hs.locked = gp
		hs.logger.Debug("Locked: ", gp)
	} else if !ok {
		return nil
	}

	ggp, ok := hs.blockchain.Get(gp.QuorumCert().BlockHash())
	// we commit the great-grandparent of the block if its grandchild is certified,
	// which we already know is true because the new block contains the grandchild's certificate,
	// and if the great-grandparent's view + 2 equals the grandchild's view.
	if ok && ggp.View()+2 == p.View() {
		return ggp
	}
	return nil
}

// ChainLength returns the number of blocks that need to be chained together in order to commit.
func (hs *SimpleHotStuff) ChainLength() int {
	return 3
}

// ProposeRule implements the default propose ruler.
func (hs *SimpleHotStuff) ProposeRule(view hotstuff.View, _ hotstuff.QuorumCert, cert hotstuff.SyncInfo, cmd *clientpb.Batch) (proposal hotstuff.ProposeMsg, ok bool) {
	qc, _ := cert.QC() // TODO: we should avoid cert does not contain a QC so we cannot fail here
	proposal = hotstuff.ProposeMsg{
		ID: hs.config.ID(),
		Block: hotstuff.NewBlock(
			qc.BlockHash(),
			qc,
			cmd,
			view,
			hs.config.ID(),
		),
	}
	if aggQC, ok := cert.AggQC(); ok && hs.config.HasAggregateQC() {
		proposal.AggregateQC = &aggQC
	}
	return proposal, true
}

var _ consensus.Ruleset = (*SimpleHotStuff)(nil)
