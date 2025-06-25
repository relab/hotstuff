package comm_test

import (
	"bytes"
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/protocol"
	"github.com/relab/hotstuff/protocol/comm"
	"github.com/relab/hotstuff/protocol/leaderrotation"
	"github.com/relab/hotstuff/protocol/votingmachine"
	"github.com/relab/hotstuff/security/crypto"
)

type leaderRotation struct{}

func (ld *leaderRotation) GetLeader(view hotstuff.View) hotstuff.ID {
	return hotstuff.ID(view) // simple leader that casts view to leader ID
}

var _ leaderrotation.LeaderRotation = (*leaderRotation)(nil)

func TestDisseminateAggregate(t *testing.T) {
	essentials := testutil.WireUpEssentials(t, 1, crypto.NameECDSA)
	viewStates, err := protocol.NewViewStates(
		essentials.BlockChain(),
		essentials.Authority(),
	)
	if err != nil {
		t.Fatal(err)
	}
	votingMachine := votingmachine.New(
		essentials.Logger(),
		essentials.EventLoop(),
		essentials.RuntimeCfg(),
		essentials.BlockChain(),
		essentials.Authority(),
		viewStates,
	)
	clique := comm.NewClique(
		essentials.RuntimeCfg(),
		votingMachine,
		&leaderRotation{},
		essentials.MockSender(),
	)
	block := testutil.CreateBlock(t, essentials.Authority())
	pc := testutil.CreatePC(t, block, essentials.Authority())
	proposal := &hotstuff.ProposeMsg{
		ID:    1,
		Block: block,
	}
	err = clique.Disseminate(proposal, pc)
	if err != nil {
		t.Fatal(err)
	}
	messages := essentials.MockSender().MessagesSent()
	if len(messages) != 1 {
		t.Fatal("expected a message to be sent")
	}
	msgProposal, ok := messages[0].(hotstuff.ProposeMsg)
	if !ok {
		t.Fatal("incorrect message type was sent")
	}
	if msgProposal.ID != proposal.ID || msgProposal.Block != proposal.Block {
		t.Fatal("incorrect message data")
	}
	// reusing the previous partial cert
	// aggregating for view 2 to change the leader to 2 so clique will aggregate instead
	// of storing the vote internally
	err = clique.Aggregate(1, nil, pc)
	if err != nil {
		t.Fatal(err)
	}
	messages = essentials.MockSender().MessagesSent()
	if len(messages) != 2 {
		t.Fatal("expected another message to be sent")
	}
	// checking the second message
	msgPC, ok := messages[1].(hotstuff.PartialCert)
	if !ok {
		t.Fatal("incorrect message type was sent")
	}
	if !bytes.Equal(msgPC.ToBytes(), pc.ToBytes()) {
		t.Fatal("incorrect message data")
	}
}
