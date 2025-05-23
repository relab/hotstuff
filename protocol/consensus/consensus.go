package consensus

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol/proposer"
	"github.com/relab/hotstuff/protocol/voter"
	"github.com/relab/hotstuff/service/committer"
)

// Consensus provides a default implementation of the Consensus interface
// for implementations of the ConsensusImpl interface.
type Consensus struct {
	eventLoop *eventloop.EventLoop
	logger    logging.Logger
	config    *core.RuntimeConfig

	committer *committer.Committer

	sender modules.ConsensusSender

	voter    *voter.Voter
	proposer *proposer.Proposer
}

// New returns a new Consensus instance based on the given Rules implementation.
func New(
	// core dependencies
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	config *core.RuntimeConfig,

	// protocol dependencies
	sender modules.ConsensusSender,
	proposer *proposer.Proposer,
	voter *voter.Voter,

	// service dependencies
	committer *committer.Committer,
) *Consensus {
	cs := &Consensus{
		eventLoop: eventLoop,
		logger:    logger,
		config:    config,

		sender:   sender,
		proposer: proposer,
		voter:    voter,

		committer: committer,
	}
	cs.eventLoop.RegisterHandler(hotstuff.ProposeMsg{}, func(event any) {
		p := event.(hotstuff.ProposeMsg)
		cs.OnPropose(&p)
	})
	return cs
}

// OnPropose is called when receiving a proposal from a leader and returns true if the proposal was voted for.
func (cs *Consensus) OnPropose(proposal *hotstuff.ProposeMsg) {
	block := proposal.Block
	// ensure that I can vote in this view based on the protocol's rule.
	err := cs.voter.Verify(proposal)
	if err != nil {
		cs.logger.Infof("failed to verify incoming vote: %v", err)
		return
	}
	cs.committer.Commit(block) // commit the valid block
	pc, err := cs.voter.Vote(block)
	if err != nil {
		cs.logger.Errorf("failed to vote: %v", err)
	}
	// send the vote and advance the view
	cs.sender.SendVote(proposal, pc)
	newInfo := hotstuff.NewSyncInfo().WithQC(block.QuorumCert())
	cs.eventLoop.AddEvent(hotstuff.NewViewMsg{
		ID:       cs.config.ID(),
		SyncInfo: newInfo,
	})
}

// Propose creates a new outgoing proposal.
func (cs *Consensus) Propose(view hotstuff.View, highQC hotstuff.QuorumCert, syncInfo hotstuff.SyncInfo) {
	proposal, err := cs.proposer.CreateProposal(view, highQC, syncInfo)
	if err != nil {
		// NOTE: errors frequently come from this method, so we just debug log here.
		cs.logger.Debugf("could not create proposal: %v", err)
		return
	}
	block := proposal.Block
	cs.committer.Commit(block) // commit the valid block
	// as proposer, I can vote for my own proposal without verifying.
	// NOTE: this vote call is not likely to fail since the leader does it, otherwise something went terribly wrong...
	pc, err := cs.voter.Vote(block)
	if err != nil {
		cs.logger.Errorf("failed to vote for my own proposal: %v", err)
	}
	// TODO(AlanRostem): moved this line to HotStuff since Kauri already sends a new view in its own logic. Check if this is valid.
	// cs.votingMachine.CollectVote(hotstuff.VoteMsg{ID: cs.config.ID(), PartialCert: pc})
	cs.sender.SendPropose(&proposal, pc)
}
