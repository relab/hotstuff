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
	blockChain   *blockchain.BlockChain
	ruler        modules.ProposeRuler
	dissAgg      modules.DisseminatorAggregator
	voter        *Voter
	commandCache *clientpb.CommandCache
	committer    *Committer

	lastProposed hotstuff.View
}

func NewProposer(
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	config *core.RuntimeConfig,
	blockChain *blockchain.BlockChain,
	dissAgg modules.DisseminatorAggregator,
	voter *Voter,
	commandCache *clientpb.CommandCache,
	committer *Committer,
	opts ...ProposerOption,
) *Proposer {
	p := &Proposer{
		eventLoop:    eventLoop,
		logger:       logger,
		config:       config,
		blockChain:   blockChain,
		ruler:        nil,
		dissAgg:      dissAgg,
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
func (p *Proposer) markProposed(view hotstuff.View, highQCBlockHash hotstuff.Hash) {
	qcBlock, ok := p.blockChain.Get(highQCBlockHash)
	if !ok {
		// NOTE: this should not occur, otherwise something went terribly wrong
		p.logger.Errorf("qcBlock not found")
		return
	}
	for qcBlock.View() > p.lastProposed {
		p.commandCache.Proposed(qcBlock.Commands()) // mark as proposed
		qc := qcBlock.QuorumCert()
		qcBlock, ok = p.blockChain.Get(qc.BlockHash())
		if !ok {
			p.logger.Errorf("qcBlock not found")
			return
		}
	}
	p.lastProposed = view
}

// Propose creates a new outgoing proposal.
func (p *Proposer) Propose(proposal *hotstuff.ProposeMsg) {
	p.logger.Debug("Propose")
	if err := p.voter.Verify(proposal); err != nil {
		p.logger.Error(err)
		return
	}
	// as proposer, I can vote for my own proposal without verifying.
	pc, err := p.voter.Vote(proposal.Block)
	if err != nil {
		p.logger.Error(err)
		return
	}
	if err := p.committer.Update(proposal.Block); err != nil {
		p.logger.Error(err)
	}
	if err := p.dissAgg.Disseminate(proposal, pc); err != nil {
		p.logger.Error(err)
	}
}

// CreateProposal attempts to create a new outgoing proposal if a command exists and the protocol's rule is satisfied.
func (p *Proposer) CreateProposal(view hotstuff.View, highQC hotstuff.QuorumCert, syncInfo hotstuff.SyncInfo) (proposal hotstuff.ProposeMsg, err error) {
	ctx, cancel := timeout.Context(p.eventLoop.Context(), p.eventLoop)
	defer cancel()
	p.markProposed(view, highQC.BlockHash())
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
