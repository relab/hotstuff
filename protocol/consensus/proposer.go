package consensus

import (
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/protocol/disagg"
	"github.com/relab/hotstuff/protocol/synchronizer/timeout"
	"github.com/relab/hotstuff/security/blockchain"
)

type Proposer struct {
	eventLoop    *eventloop.EventLoop
	config       *core.RuntimeConfig
	blockchain   *blockchain.Blockchain
	ruler        ProposeRuler
	disseminator disagg.Disseminator
	voter        *Voter
	commandCache *clientpb.CommandCache
	committer    *Committer

	lastProposed hotstuff.View
}

func NewProposer(
	eventLoop *eventloop.EventLoop,
	config *core.RuntimeConfig,
	blockchain *blockchain.Blockchain,
	ruler ProposeRuler,
	disseminator disagg.Disseminator,
	voter *Voter,
	commandCache *clientpb.CommandCache,
	committer *Committer,
) *Proposer {
	return &Proposer{
		eventLoop:    eventLoop,
		config:       config,
		blockchain:   blockchain,
		ruler:        ruler,
		disseminator: disseminator,
		voter:        voter,
		commandCache: commandCache,
		committer:    committer,

		lastProposed: 0, // genesis block has zero view
	}
}

// markProposed traverses the block history and marks commands as proposed.
func (p *Proposer) markProposed(view hotstuff.View, highQCBlockHash hotstuff.Hash) error {
	qcBlock, ok := p.blockchain.Get(highQCBlockHash)
	if !ok {
		// NOTE: this should not occur, otherwise something went terribly wrong
		return fmt.Errorf(
			"failed to mark proposed: block not found for high QC block hash: %s",
			highQCBlockHash.SmallString())
	}
	for qcBlock.View() > p.lastProposed {
		p.commandCache.Proposed(qcBlock.Commands()) // mark as proposed
		qc := qcBlock.QuorumCert()
		qcBlock, ok = p.blockchain.Get(qc.BlockHash())
		if !ok {
			return fmt.Errorf(
				"failed to mark proposed: qcBlock not found: %s",
				qcBlock.Hash().SmallString())
		}
	}
	p.lastProposed = view
	return nil
}

// Propose creates a new outgoing proposal.
func (p *Proposer) Propose(proposal *hotstuff.ProposeMsg) error {
	if err := p.voter.Verify(proposal); err != nil {
		return err
	}
	// as proposer, I can vote for my own proposal without verifying.
	pc, err := p.voter.Vote(proposal.Block)
	if err != nil {
		return err
	}
	if err := p.committer.TryCommit(proposal.Block); err != nil {
		return err
	}
	if err := p.disseminator.Disseminate(proposal, pc); err != nil {
		return err
	}
	return nil
}

// CreateProposal attempts to create a new outgoing proposal if a command exists and the protocol's rule is satisfied.
func (p *Proposer) CreateProposal(view hotstuff.View, highQC hotstuff.QuorumCert, syncInfo hotstuff.SyncInfo) (proposal hotstuff.ProposeMsg, err error) {
	ctx, cancel := timeout.Context(p.eventLoop.Context(), p.eventLoop)
	defer cancel()
	if err := p.markProposed(view, highQC.BlockHash()); err != nil {
		return proposal, err
	}
	// TODO(meling): Should this return a partially filled batch if there is a timeout? What is the timeout? Right now, it returns nil if ctx is canceled.
	// TODO(meling): Note: the ctx is canceled on view change as well; should it return a batch on view change?
	// find a value to propose.
	// NOTE: this is blocking until a batch is present in the cache.
	cmdBatch, err := p.commandCache.Get(ctx)
	if err != nil {
		return proposal, fmt.Errorf("no command batch: %w", err)
	}
	// ensure that a proposal can be sent based on the protocol's rule.
	// NOTE: the ruler will create the proposal too.
	proposal, ok := p.ruler.ProposeRule(view, highQC, syncInfo, cmdBatch)
	if !ok {
		return proposal, fmt.Errorf("propose rule not satisfied")
	}
	return
}
