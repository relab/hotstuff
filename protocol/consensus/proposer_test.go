package consensus_test

import (
	"bytes"
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

func check(t *testing.T, err error) {
	if err != nil {
		t.Fatal(err)
	}
}

func wireUpProposer(
	t *testing.T,
	essentials *testutil.Essentials,
	commandCache *clientpb.CommandCache,
) (*consensus.Proposer, *protocol.ViewStates) {
	t.Helper()
	consensusRules := rules.NewChainedHotStuff(
		essentials.Logger(),
		essentials.RuntimeCfg(),
		essentials.BlockChain(),
	)
	viewStates, err := protocol.NewViewStates(
		essentials.BlockChain(),
		essentials.Authority(),
	)
	check(t, err)
	leaderRotation := leaderrotation.NewFixed(1)
	votingMachine := votingmachine.New(
		essentials.Logger(),
		essentials.EventLoop(),
		essentials.RuntimeCfg(),
		essentials.BlockChain(),
		essentials.Authority(),
		viewStates,
	)
	comm := comm.NewClique(
		essentials.RuntimeCfg(),
		votingMachine,
		leaderRotation,
		essentials.MockSender(),
	)
	committer := consensus.NewCommitter(
		essentials.EventLoop(),
		essentials.Logger(),
		essentials.BlockChain(),
		viewStates,
		consensusRules,
	)
	voter := consensus.NewVoter(
		essentials.RuntimeCfg(),
		leaderRotation,
		consensusRules,
		comm,
		essentials.Authority(),
		committer,
	)
	return consensus.NewProposer(
		essentials.EventLoop(),
		essentials.RuntimeCfg(),
		essentials.BlockChain(),
		viewStates,
		consensusRules,
		comm,
		voter,
		commandCache,
		committer,
	), viewStates
}

func TestPropose(t *testing.T) {
	id := hotstuff.ID(1)
	essentials := testutil.WireUpEssentials(t, id, crypto.NameECDSA)
	// add the blockchains to the proposer's mock sender
	essentials.MockSender().AddBlockChain(essentials.BlockChain())
	command := &clientpb.Command{
		ClientID:       1,
		SequenceNumber: 1,
		Data:           []byte("testing data here"),
	}
	commandCache := clientpb.NewCommandCache(1)
	commandCache.Add(command)
	proposer, viewStates := wireUpProposer(t, essentials, commandCache)
	highQC := testutil.CreateQC(t, hotstuff.GetGenesis(), essentials.Authority())
	_, err := viewStates.UpdateHighQC(highQC)
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := proposer.CreateProposal(hotstuff.NewSyncInfo().WithQC(highQC))
	if err != nil {
		t.Fatal(err)
	}
	if err := proposer.Propose(&proposal); err != nil {
		t.Fatal(err)
	}
	messages := essentials.MockSender().MessagesSent()
	if len(messages) != 1 {
		t.Fatal("expected at least one message to be sent by proposer")
	}
	msg := messages[0]
	if _, ok := msg.(hotstuff.ProposeMsg); !ok {
		t.Fatal("expected message to be hotstuff.ProposeMsg")
	}
	proposeMsg := msg.(hotstuff.ProposeMsg)
	if proposeMsg.ID != 1 || !bytes.Equal(proposeMsg.Block.ToBytes(), proposal.Block.ToBytes()) {
		t.Fatal("incorrect propose message data")
	}
}
