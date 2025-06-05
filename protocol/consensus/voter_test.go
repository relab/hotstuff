package consensus_test

import (
	"context"
	"testing"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/protocol/committer"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/leaderrotation/fixedleader"
	"github.com/relab/hotstuff/protocol/rules/chainedhotstuff"
	"github.com/relab/hotstuff/protocol/synchronizer/viewduration"
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
) (*consensus.Voter, error) {
	t.Helper()
	consensusRules, err := wiring.NewConsensusRules(
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		depsSecurity.BlockChain(),
		list.consensusRules,
	)
	if err != nil {
		return nil, err
	}
	committer := committer.New(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsSecurity.BlockChain(),
		consensusRules,
	)
	leaderRotation, err := wiring.NewLeaderRotation(
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		depsSecurity.BlockChain(),
		committer,
		viewduration.NewParams(1, 100*time.Millisecond, 0, 1.2),
		list.leaderRotation,
		consensusRules.ChainLength(),
	)
	if err != nil {
		return nil, err
	}
	viewStates, err := consensus.NewViewStates(
		depsSecurity.BlockChain(),
		depsSecurity.Authority(),
	)
	if err != nil {
		return nil, err
	}
	hsProtocol := consensus.NewHotStuff(
		depsCore.Logger(),
		depsCore.EventLoop(),
		depsCore.RuntimeCfg(),
		depsSecurity.BlockChain(),
		depsSecurity.Authority(),
		viewStates,
		leaderRotation,
		sender,
	)
	commandCache := clientpb.New()
	voter := consensus.NewVoter(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		leaderRotation,
		consensusRules,
		hsProtocol,
		depsSecurity.Authority(),
		commandCache,
		committer,
	)
	return voter, nil
}

// TestOnValidPropose checks that a voter will advance the view when receiving a valid proposal.
func TestOnValidPropose(t *testing.T) {
	id := hotstuff.ID(1)
	depsCore := wiring.NewCore(id, "test", testutil.GenerateECDSAKey(t))
	newViewTriggered := false
	depsCore.EventLoop().RegisterHandler(hotstuff.NewViewMsg{}, func(_ any) {
		newViewTriggered = true
	})
	sender := testutil.NewMockSender(depsCore.RuntimeCfg().ID())
	// TODO(AlanRostem): put this in some test data
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
	voter, err := wireUpVoter(t, depsCore, depsSecurity, sender, list)
	if err != nil {
		t.Fatal(err)
	}
	// create a block signed by self and vote for it
	block := testutil.CreateBlock(t, depsSecurity.Authority())
	proposal := hotstuff.ProposeMsg{
		ID:    id,
		Block: block,
	}
	if err := voter.Verify(&proposal); err != nil {
		t.Fatalf("could not verify proposal: %v", err)
	}
	voter.OnValidPropose(&proposal)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	depsCore.EventLoop().Run(ctx)
	if !newViewTriggered {
		t.Fatal("the voter did not advance the view")
	}
	if voter.LastVote() != proposal.Block.View() {
		t.Fatal("incorrect view voted for")
	}
}
