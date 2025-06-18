package consensus

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol"
	"github.com/relab/hotstuff/security/blockchain"
	"github.com/relab/hotstuff/security/cert"
)

type HotStuff struct {
	config         *core.RuntimeConfig
	votingMachine  *VotingMachine
	leaderRotation modules.LeaderRotation
	sender         modules.Sender
}

func NewHotStuff(
	logger logging.Logger,
	eventLoop *eventloop.EventLoop,
	config *core.RuntimeConfig,
	blockchain *blockchain.Blockchain,
	auth *cert.Authority,
	states *protocol.ViewStates,
	leaderRotation modules.LeaderRotation,
	sender modules.Sender,
) *HotStuff {
	return &HotStuff{
		config: config,
		votingMachine: NewVotingMachine(
			logger,
			eventLoop,
			config,
			blockchain,
			auth,
			states,
		),
		leaderRotation: leaderRotation,
		sender:         sender,
	}
}

// Disseminate stores a vote for the proposal and broadcasts the proposal.
func (hs *HotStuff) Disseminate(proposal *hotstuff.ProposeMsg, pc hotstuff.PartialCert) error {
	hs.votingMachine.CollectVote(hotstuff.VoteMsg{
		ID:          hs.config.ID(),
		PartialCert: pc,
	})
	hs.sender.Propose(proposal)
	return nil
}

// Aggregate aggregates the vote or stores it if the replica is leader in the next view.
func (hs *HotStuff) Aggregate(lastVote hotstuff.View, _ *hotstuff.ProposeMsg, pc hotstuff.PartialCert) error {
	leaderID := hs.leaderRotation.GetLeader(lastVote + 1)
	if leaderID == hs.config.ID() {
		// if I am the leader in the next view, collect the vote for myself beforehand.
		hs.votingMachine.CollectVote(hotstuff.VoteMsg{
			ID:          hs.config.ID(),
			PartialCert: pc,
		})
		return nil
	}
	// if I am the one voting, send the vote to next leader over the wire.
	return hs.sender.Vote(leaderID, pc)
}

var _ modules.DisseminatorAggregator = (*HotStuff)(nil)
