package consensus_test

import (
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/protocol"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/leaderrotation/fixedleader"
	"github.com/relab/hotstuff/protocol/rules/chainedhotstuff"
	"github.com/relab/hotstuff/security/crypto/ecdsa"
	"github.com/relab/hotstuff/wiring"
)

type moduleList struct {
	consensusRules string
	leaderRotation string
	cryptoBase     string
}

func wireUpVoter(
	t *testing.T,
	depsCore *wiring.Core,
	depsSecurity *wiring.Security,
	sender *testutil.MockSender,
	list moduleList,
) *consensus.Voter {
	t.Helper()
	consensusRules, err := wiring.NewConsensusRules(
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		depsSecurity.BlockChain(),
		list.consensusRules,
	)
	check(t, err)
	viewStates, err := protocol.NewViewStates(
		depsSecurity.BlockChain(),
		depsSecurity.Authority(),
	)
	check(t, err)
	committer := consensus.NewCommitter(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsSecurity.BlockChain(),
		viewStates,
		consensusRules,
	)
	leaderRotation := fixedleader.New(2) // want a leader that is not 1
	votingMachine := consensus.NewVotingMachine(
		depsCore.Logger(),
		depsCore.EventLoop(),
		depsCore.RuntimeCfg(),
		depsSecurity.BlockChain(),
		depsSecurity.Authority(),
		viewStates,
	)
	hsDissAgg := consensus.NewHotStuff(
		depsCore.RuntimeCfg(),
		votingMachine,
		leaderRotation,
		sender,
	)
	voter := consensus.NewVoter(
		depsCore.RuntimeCfg(),
		leaderRotation,
		consensusRules,
		hsDissAgg,
		depsSecurity.Authority(),
		committer,
	)
	return voter
}

// TestOnValidPropose checks that a voter will aggregate a partial cert when receiving a valid proposal from
// an honest leader.
func TestOnValidPropose(t *testing.T) {
	id := hotstuff.ID(1)
	depsCore := wiring.NewCore(id, "test", testutil.GenerateECDSAKey(t))
	sender := testutil.NewMockSender(depsCore.RuntimeCfg().ID())
	list := moduleList{
		consensusRules: chainedhotstuff.ModuleName,
		leaderRotation: fixedleader.ModuleName,
		cryptoBase:     ecdsa.ModuleName,
	}
	depsSecurity, err := wiring.NewSecurity(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		sender,
		list.cryptoBase,
	)
	if err != nil {
		t.Fatal(err)
	}
	sender.AddBlockChain(depsSecurity.BlockChain())
	voter := wireUpVoter(t, depsCore, depsSecurity, sender, list)
	// create a signed block (doesn't matter who did it)
	qc, err := depsSecurity.Authority().CreateQuorumCert(hotstuff.GetGenesis(), []hotstuff.PartialCert{})
	if err != nil {
		t.Fatal(err)
	}
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
	if voter.LastVote() != proposal.Block.View() {
		t.Fatal("incorrect view voted for")
	}
}
