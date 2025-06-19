package consensus

import (
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol/synchronizer/timeout"
	"github.com/relab/hotstuff/security/blockchain"
)

type Proposer struct {
	eventLoop    *eventloop.EventLoop
	logger       logging.Logger
	config       *core.RuntimeConfig
	blockchain   *blockchain.Blockchain
	ruler        modules.ProposeRuler
	disseminator modules.Disseminator
	voter        *Voter
	commandCache *clientpb.CommandCache
	committer    *Committer

	lastProposed hotstuff.View
}

func NewProposer(
	eventLoop *eventloop.EventLoop,
	config *core.RuntimeConfig,
	blockchain *blockchain.Blockchain,
	disseminator modules.Disseminator,
	voter *Voter,
	commandCache *clientpb.CommandCache,
	committer *Committer,
	opts ...ProposerOption,
) *Proposer {
	p := &Proposer{
		eventLoop:    eventLoop,
		config:       config,
		blockchain:   blockchain,
		ruler:        nil,
		disseminator: disseminator,
		voter:        voter,
		commandCache: commandCache,
		committer:    committer,

		lastProposed: 0, // genesis block has zero view
	}
	p.ruler = p
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// markProposed traverses the block history and marks commands as proposed.
func (p *Proposer) markProposed(view hotstuff.View, highQCBlockHash hotstuff.Hash) error {
	qcBlock, ok := p.blockchain.Get(highQCBlockHash)
	if !ok {
		// NOTE: this should not occur, otherwise something went terribly wrong
		return fmt.Errorf("failed to mark proposed: block not found for high QC block hash: %s", highQCBlockHash.String())
	}
	for qcBlock.View() > p.lastProposed {
		p.commandCache.Proposed(qcBlock.Commands()) // mark as proposed
		qc := qcBlock.QuorumCert()
		qcBlock, ok = p.blockchain.Get(qc.BlockHash())
		if !ok {
			return fmt.Errorf("failed to mark proposed: qcBlock not found: %s", qcBlock.Hash().String())
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
		return proposal, fmt.Errorf("no command batch: %v", err)
	}
	// ensure that a proposal can be sent based on the protocol's rule.
	// NOTE: the ruler will create the proposal too.
	proposal, ok := p.ruler.ProposeRule(view, highQC, syncInfo, cmdBatch)
	if !ok {
		return proposal, fmt.Errorf("propose rule not satisfied")
	}
	return
}

// ProposeRule implements the default propose ruler.
func (p *Proposer) ProposeRule(view hotstuff.View, _ hotstuff.QuorumCert, cert hotstuff.SyncInfo, cmd *clientpb.Batch) (proposal hotstuff.ProposeMsg, ok bool) {
	qc, _ := cert.QC() // TODO: we should avoid cert does not contain a QC so we cannot fail here
	proposal = hotstuff.ProposeMsg{
		ID: p.config.ID(),
		Block: hotstuff.NewBlock(
			qc.BlockHash(),
			qc,
			cmd,
			view,
			p.config.ID(),
		),
	}
	if aggQC, ok := cert.AggQC(); ok && p.config.HasAggregateQC() {
		proposal.AggregateQC = &aggQC
	}
	return proposal, true
}

var _ modules.ProposeRuler = (*Proposer)(nil)
