package consensus

import (
	"fmt"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/proto/clientpb"
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

	committer    *committer.Committer
	commandCache *clientpb.Cache

	protocol modules.ConsensusProtocol

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
	protocol modules.ConsensusProtocol,
	proposer *proposer.Proposer,
	voter *voter.Voter,

	// service dependencies
	commandCache *clientpb.Cache,
	committer *committer.Committer,
) *Consensus {
	cs := &Consensus{
		eventLoop: eventLoop,
		logger:    logger,
		config:    config,

		protocol:     protocol,
		proposer:     proposer,
		voter:        voter,
		commandCache: commandCache,

		committer: committer,
	}
	cs.eventLoop.RegisterHandler(hotstuff.ProposeMsg{}, func(event any) {
		p := event.(hotstuff.ProposeMsg)
		cs.OnPropose(&p)
	})
	return cs
}

func (cs *Consensus) internalVote(block *hotstuff.Block) (pc hotstuff.PartialCert, err error) {
	// store the valid block, it may commit the block or its ancestors
	cs.committer.Update(block)
	// TODO(AlanRostem): solve issue #191
	// update the command's age before voting.
	cs.commandCache.Proposed(block.Command())
	pc, err = cs.voter.Vote(block)
	if err != nil {
		// if the block is invalid, reject it. This means the command is also discarded.
		return pc, fmt.Errorf("failed to vote: %v", err)
	}
	return pc, nil
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
	pc, err := cs.internalVote(block)
	if err != nil {
		cs.logger.Infof("%v", err)
	} else {
		// send the vote if it was successful
		cs.protocol.SendVote(proposal, pc)
	}
	// advance the view regardless of vote success/failure
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
		// do not send or vote interally for the proposal if it could not be created successfully
		cs.logger.Debugf("could not create proposal: %v", err)
		return
	}
	block := proposal.Block
	pc, err := cs.internalVote(block)
	if err != nil {
		// this is never going to happen, but it's nice to log in case of a bug
		cs.logger.Errorf("critical error at proposer: %v", err)
		return
	}
	// TODO(AlanRostem): moved this line to HotStuff since Kauri already sends a new view in its own logic. Check if this is valid.
	// cs.votingMachine.CollectVote(hotstuff.VoteMsg{ID: cs.config.ID(), PartialCert: pc})
	// as proposer, I can vote for my own proposal without verifying.
	cs.protocol.SendPropose(&proposal, pc)
}
