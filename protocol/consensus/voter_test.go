package consensus_test

import (
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/protocol"
	"github.com/relab/hotstuff/protocol/comm"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/leaderrotation"
	"github.com/relab/hotstuff/protocol/rules"
	"github.com/relab/hotstuff/protocol/votingmachine"
	"github.com/relab/hotstuff/security/crypto"
)

func wireUpVoter(
	t *testing.T,
	essentials *testutil.Essentials,
) *consensus.Voter {
	t.Helper()
	consensusRules := rules.NewChainedHotStuff(
		essentials.Logger(),
		essentials.RuntimeCfg(),
		essentials.Blockchain(),
	)
	viewStates, err := protocol.NewViewStates(
		essentials.Blockchain(),
		essentials.Authority(),
	)
	check(t, err)
	committer := consensus.NewCommitter(
		essentials.EventLoop(),
		essentials.Logger(),
		essentials.Blockchain(),
		viewStates,
		consensusRules,
	)
	leaderRotation := leaderrotation.NewFixed(2) // want a leader that is not 1 in this test case
	votingMachine := votingmachine.New(
		essentials.Logger(),
		essentials.EventLoop(),
		essentials.RuntimeCfg(),
		essentials.Blockchain(),
		essentials.Authority(),
		viewStates,
	)
	comm := comm.NewClique(
		essentials.RuntimeCfg(),
		votingMachine,
		leaderRotation,
		essentials.MockSender(),
	)
	voter := consensus.NewVoter(
		essentials.RuntimeCfg(),
		leaderRotation,
		consensusRules,
		comm,
		essentials.Authority(),
		committer,
	)
	return voter
}

// TestOnValidPropose checks that a voter will aggregate a partial cert when receiving a valid proposal from
// an honest leader.
func TestOnValidPropose(t *testing.T) {
	id := hotstuff.ID(1)
	essentials := testutil.WireUpEssentials(t, id, crypto.NameECDSA)

	voter := wireUpVoter(t, essentials)
	// create a signed block (doesn't matter who did it)
	qc := testutil.CreateQC(t, hotstuff.GetGenesis(), essentials.Authority())
	// create a propose message with a valid block from a replica who is not 1
	proposerID := hotstuff.ID(2)
	block := hotstuff.NewBlock(
		hotstuff.GetGenesis().Hash(),
		qc,
		&clientpb.Batch{},
		1,
		proposerID,
	)
	proposal := hotstuff.ProposeMsg{
		ID:    proposerID,
		Block: block,
	}
	// verify proposal
	if err := voter.Verify(&proposal); err != nil {
		t.Fatalf("could not verify proposal: %v", err)
	}
	// process proposal (vote happens here)
	if err := voter.OnValidPropose(&proposal); err != nil {
		t.Fatalf("failure to process proposal: %v", err)
	}
	sender := essentials.MockSender()
	// check vote aggregation
	if len(sender.MessagesSent()) != 1 {
		t.Fatal("no vote was aggregated")
	}
	// validate message data
	pc, ok := sender.MessagesSent()[0].(hotstuff.PartialCert)
	if !ok {
		t.Fatal("incorrect message type was sent")
	}
	if pc.BlockHash().String() != block.Hash().String() {
		t.Fatal("incorrect partial cert was aggregated")
	}
	// verify that voting was stopped for the view after voting
	if voter.StopVoting(proposal.Block.View()) {
		t.Fatal("should not be able to advance to the same view we just voted for")
	}
}

func TestStopVoting(t *testing.T) {
	tests := []struct {
		name string
		view hotstuff.View
		want bool
	}{
		{name: "advance from view 0 to view 1", view: 1, want: true},
		{name: "stop voting for same view", view: 1, want: false},
		{name: "stop voting for earlier view", view: 0, want: false},
		{name: "advance to view 2", view: 2, want: true},
		{name: "advance to higher view", view: 5, want: true},
		{name: "stop voting for view less than current", view: 3, want: false},
		{name: "stop voting for current view", view: 5, want: false},
		{name: "advance to much higher view", view: 10, want: true},
	}

	id := hotstuff.ID(1)
	essentials := testutil.WireUpEssentials(t, id, crypto.NameECDSA)
	voter := wireUpVoter(t, essentials)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := voter.StopVoting(tt.view)
			if got != tt.want {
				t.Errorf("StopVoting(%d) = %v, want %v", tt.view, got, tt.want)
			}
		})
	}
}
