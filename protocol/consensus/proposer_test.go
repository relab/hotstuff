package consensus_test

import (
	"bytes"
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/protocol"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/disagg/clique"
	"github.com/relab/hotstuff/protocol/leaderrotation/fixedleader"
	"github.com/relab/hotstuff/protocol/rules/chainedhotstuff"
	"github.com/relab/hotstuff/protocol/votingmachine"
	"github.com/relab/hotstuff/security/crypto/ecdsa"
	"github.com/relab/hotstuff/wiring"
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
) *consensus.Proposer {
	t.Helper()
	consensusRules := chainedhotstuff.New(
		essentials.Logger(),
		essentials.BlockChain(),
	)
	viewStates, err := protocol.NewViewStates(
		essentials.BlockChain(),
		essentials.Authority(),
	)
	check(t, err)
	leaderRotation := fixedleader.New(1)
	votingMachine := votingmachine.New(
		essentials.Logger(),
		essentials.EventLoop(),
		essentials.RuntimeCfg(),
		essentials.BlockChain(),
		essentials.Authority(),
		viewStates,
	)
	disAgg := clique.New(
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
		disAgg,
		essentials.Authority(),
		committer,
	)
	return consensus.NewProposer(
		essentials.EventLoop(),
		essentials.RuntimeCfg(),
		essentials.BlockChain(),
		disAgg,
		voter,
		commandCache,
		committer,
	)
}

type replica struct {
	depsCore     *wiring.Core
	depsSecurity *wiring.Security
	sender       *testutil.MockSender
}

func TestPropose(t *testing.T) {
	id := hotstuff.ID(1)
	essentials := testutil.WireUpEssentials(t, id, ecdsa.ModuleName)
	// add the blockchains to the proposer's mock sender
	essentials.MockSender().AddBlockChain(essentials.BlockChain())
	command := &clientpb.Command{
		ClientID:       1,
		SequenceNumber: 1,
		Data:           []byte("testing data here"),
	}
	commandCache := clientpb.NewCommandCache(1)
	commandCache.Add(command)
	proposer := wireUpProposer(t, essentials, commandCache)
	highQC := testutil.CreateQC(t, hotstuff.GetGenesis(), essentials.Authority())
	proposal, err := proposer.CreateProposal(1, highQC, hotstuff.NewSyncInfo().WithQC(highQC))
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
