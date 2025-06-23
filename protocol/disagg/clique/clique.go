package clique

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/protocol/disagg"
	"github.com/relab/hotstuff/protocol/leaderrotation"
	"github.com/relab/hotstuff/protocol/votingmachine"
)

// Clique implements one-to-all dissemination and all-to-one aggregation.
// TODO(AlanRostem): make tests
type Clique struct {
	config         *core.RuntimeConfig
	votingMachine  *votingmachine.VotingMachine
	leaderRotation leaderrotation.LeaderRotation
	sender         core.Sender
}

func New(
	config *core.RuntimeConfig,
	votingMachine *votingmachine.VotingMachine,
	leaderRotation leaderrotation.LeaderRotation,
	sender core.Sender,
) *Clique {
	return &Clique{
		config:         config,
		votingMachine:  votingMachine,
		leaderRotation: leaderRotation,
		sender:         sender,
	}
}

// Disseminate stores a vote for the proposal and broadcasts the proposal.
func (hs *Clique) Disseminate(proposal *hotstuff.ProposeMsg, pc hotstuff.PartialCert) error {
	hs.votingMachine.CollectVote(hotstuff.VoteMsg{
		ID:          hs.config.ID(),
		PartialCert: pc,
	})
	hs.sender.Propose(proposal)
	return nil
}

// Aggregate aggregates the vote or stores it if the replica is leader in the next view.
func (hs *Clique) Aggregate(lastVote hotstuff.View, _ *hotstuff.ProposeMsg, pc hotstuff.PartialCert) error {
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

var _ disagg.Aggregator = (*Clique)(nil)
var _ disagg.Disseminator = (*Clique)(nil)
