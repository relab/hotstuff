package consensus

import (
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol/committer"
	"github.com/relab/hotstuff/protocol/synchronizer/timeout"
)

type Proposer struct {
	eventLoop    *eventloop.EventLoop
	logger       logging.Logger
	config       *core.RuntimeConfig
	ruler        modules.ProposeRuler
	protocol     modules.ConsensusProtocol
	voter        *Voter
	commandCache *clientpb.Cache
	committer    *committer.Committer
}

func NewProposer(
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	config *core.RuntimeConfig,
	protocol modules.ConsensusProtocol,
	voter *Voter,
	commandCache *clientpb.Cache,
	committer *committer.Committer,
	opts ...ProposerOption,
) *Proposer {
	p := &Proposer{
		eventLoop:    eventLoop,
		logger:       logger,
		config:       config,
		ruler:        nil,
		protocol:     protocol,
		voter:        voter,
		commandCache: commandCache,
		committer:    committer,
	}
	p.ruler = p
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Propose creates a new outgoing proposal.
func (cs *Proposer) Propose(proposal *hotstuff.ProposeMsg) {
	block := proposal.Block
	// store the valid block, it may commit the block or its ancestors
	cs.committer.Update(block)
	// TODO(AlanRostem): solve issue #191
	// update the command's age before voting.
	cs.commandCache.Proposed(block.Commands())
	pc, err := cs.voter.Vote(block)
	if err != nil {
		// this should not happen which is why we log here just in case of a bug
		cs.logger.Errorf("critical: %v", err)
		return
	}
	// TODO(AlanRostem): moved this line to HotStuff since Kauri already sends a new view in its own logic. Check if this is valid.
	// cs.votingMachine.CollectVote(hotstuff.VoteMsg{ID: cs.config.ID(), PartialCert: pc})
	// as proposer, I can vote for my own proposal without verifying.
	cs.protocol.SendPropose(proposal, pc)
}

// CreateProposal attempts to create a new outgoing proposal if a command exists and the protocol's rule is satisfied.
func (cs *Proposer) CreateProposal(view hotstuff.View, highQC hotstuff.QuorumCert, syncInfo hotstuff.SyncInfo) (proposal hotstuff.ProposeMsg, err error) {
	ctx, cancel := timeout.Context(cs.eventLoop.Context(), cs.eventLoop)
	defer cancel()
	// find a value to propose.
	// NOTE: this is blocking until a batch is present in the cache.
	// TODO(meling): Should this return a partially filled batch if there is a timeout? What is the timeout? Right now, it returns nil if ctx is canceled.
	// TODO(meling): Note: the ctx is canceled on view change as well; should it return a batch on view change?
	cmdBatch, err := cs.commandCache.Get(ctx)
	if err != nil {
		return proposal, fmt.Errorf("no command batch: %v", err)
	}
	// ensure that a proposal can be sent based on the protocol's rule.
	// NOTE: the ruler will create the proposal too.
	proposal, ok := cs.ruler.ProposeRule(view, highQC, syncInfo, cmdBatch)
	if !ok {
		return proposal, fmt.Errorf("propose rule not satisfied")
	}
	return
}

// ProposeRule implements the default propose ruler.
func (cs *Proposer) ProposeRule(view hotstuff.View, _ hotstuff.QuorumCert, cert hotstuff.SyncInfo, cmd *clientpb.Batch) (proposal hotstuff.ProposeMsg, ok bool) {
	qc, _ := cert.QC() // TODO: we should avoid cert does not contain a QC so we cannot fail here
	proposal = hotstuff.ProposeMsg{
		ID: cs.config.ID(),
		Block: hotstuff.NewBlock(
			qc.BlockHash(),
			qc,
			cmd,
			view,
			cs.config.ID(),
		),
	}
	if aggQC, ok := cert.AggQC(); ok && cs.config.HasAggregateQC() {
		proposal.AggregateQC = &aggQC
	}
	return proposal, true
}

var _ modules.ProposeRuler = (*Proposer)(nil)
