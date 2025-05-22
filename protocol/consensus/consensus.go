package consensus

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol/proposer"
	"github.com/relab/hotstuff/protocol/voter"
	"github.com/relab/hotstuff/protocol/votingmachine"
	"github.com/relab/hotstuff/service/committer"
)

// Consensus provides a default implementation of the Consensus interface
// for implementations of the ConsensusImpl interface.
type Consensus struct {
	eventLoop *eventloop.EventLoop
	logger    logging.Logger
	config    *core.RuntimeConfig

	committer *committer.Committer

	leaderRotation modules.LeaderRotation
	extHandler     modules.ExtProposeHandler

	voter         *voter.Voter
	votingMachine *votingmachine.VotingMachine
	proposer      *proposer.Proposer

	sender modules.Sender
}

// New returns a new Consensus instance based on the given Rules implementation.
func New(
	// core dependencies
	eventLoop *eventloop.EventLoop,
	logger logging.Logger,
	config *core.RuntimeConfig,

	// protocol dependencies
	leaderRotation modules.LeaderRotation,
	proposer *proposer.Proposer,
	voter *voter.Voter,
	votingMachine *votingmachine.VotingMachine,

	// service dependencies
	committer *committer.Committer,

	// network dependencies
	sender modules.Sender,

	// options
	opts ...Option,
) *Consensus {
	cs := &Consensus{
		leaderRotation: leaderRotation,
		committer:      committer,
		eventLoop:      eventLoop,
		logger:         logger,
		config:         config,
		sender:         sender,
		proposer:       proposer,

		voter:         voter,
		votingMachine: votingMachine,
	}
	cs.extHandler = cs
	cs.eventLoop.RegisterHandler(hotstuff.ProposeMsg{}, func(event any) {
		cs.OnPropose(event.(hotstuff.ProposeMsg))
	})

	for _, opt := range opts {
		opt(cs)
	}
	return cs
}

// OnPropose is called when receiving a proposal from a leader and returns true if the proposal was voted for.
func (cs *Consensus) OnPropose(proposal hotstuff.ProposeMsg) {
	block := proposal.Block
	// ensure that I can vote in this view based on the protocol's rule.
	err := cs.voter.Verify(&proposal)
	if err != nil {
		cs.logger.Errorf("failed to verify incoming vote: %v", err)
		return
	}
	// if we can't commit the block yet, don't vote for it.
	if !cs.committer.TryCommit(block) {
		return
	}
	// even if voting fails, we should be able to go to the next view.
	newInfo := hotstuff.NewSyncInfo().WithQC(block.QuorumCert())
	cs.eventLoop.AddEvent(hotstuff.NewViewMsg{
		ID:       cs.config.ID(),
		SyncInfo: newInfo,
	})
	// try to vote for the block and retrieve its partial certificate.
	pc, err := cs.voter.Vote(block)
	// don't send the vote if it failed
	if err != nil {
		cs.logger.Errorf("failed to vote: %v", err)
		return
	}
	cs.extHandler.ExtOnPropose(proposal, pc)
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
	// as proposer, I can vote for my own proposal without verifying.
	// NOTE: this vote call is not likely to fail since the leader does it, otherwise something went terribly wrong...
	pc, err := cs.voter.Vote(block)
	if err != nil {
		cs.logger.Errorf("failed to vote for my own proposal: %v", err)
		return
	}
	// can collect my own vote as leader
	cs.votingMachine.CollectVote(hotstuff.VoteMsg{ID: cs.config.ID(), PartialCert: pc})
	cs.extHandler.ExtDisseminatePropose(proposal, pc)
}

func (cs *Consensus) ExtDisseminatePropose(proposal hotstuff.ProposeMsg, _ hotstuff.PartialCert) {
	cs.sender.Propose(proposal)
}

func (cs *Consensus) ExtOnPropose(proposal hotstuff.ProposeMsg, pc hotstuff.PartialCert) {
	leaderID := cs.leaderRotation.GetLeader(cs.voter.LastVote() + 1)
	if leaderID == cs.config.ID() {
		// if I am the leader in the next view, collect the vote for myself beforehand.
		cs.votingMachine.CollectVote(hotstuff.VoteMsg{ID: cs.config.ID(), PartialCert: pc})
		return
	}
	// if I am the one voting, send the vote to next leader over the wire.
	err := cs.sender.Vote(leaderID, pc)
	if err != nil {
		cs.logger.Warnf("%v", err)
		return
	}
	cs.logger.Debugf("voting for %v", proposal)
}

var _ modules.ExtProposeHandler = (*Consensus)(nil)
