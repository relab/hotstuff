package comm

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/protocol/leaderrotation"
	"github.com/relab/hotstuff/protocol/votingmachine"
)

const NameClique = "clique"

// Clique implements one-to-all dissemination and all-to-one aggregation.
type Clique struct {
	config         *core.RuntimeConfig
	votingMachine  *votingmachine.VotingMachine
	leaderRotation leaderrotation.LeaderRotation
	sender         core.Sender
}

// NewClique creates a new Clique instance for communicating proposals and votes.
func NewClique(
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

// Disseminate broadcasts the proposal and sends my vote for this proposal to the next leader
func (hs *Clique) Disseminate(proposal *hotstuff.ProposeMsg, pc hotstuff.PartialCert) error {
	hs.sender.Propose(proposal)

	leaderID := hs.leaderRotation.GetLeader(proposal.Block.View() + 1)
	if leaderID == hs.config.ID() {
		// I am the next leader, store not send
		hs.votingMachine.CollectVote(hotstuff.VoteMsg{
			ID:          hs.config.ID(),
			PartialCert: pc,
		})
	} else {
		hs.sender.Vote(leaderID, pc)
	}
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

var (
	_ Aggregator   = (*Clique)(nil)
	_ Disseminator = (*Clique)(nil)
)
