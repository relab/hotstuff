package votingmachine

import (
	"context"
	"testing"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/protocol"
	"github.com/relab/hotstuff/security/crypto/ecdsa"
)

func TestCollectVote(t *testing.T) {
	signers := testutil.NewEssentialsSet(t, 4, ecdsa.ModuleName)
	leader := signers[0]
	viewStates, err := protocol.NewViewStates(
		leader.BlockChain(),
		leader.Authority(),
	)
	if err != nil {
		t.Fatal(err)
	}
	votingMachine := New(
		leader.Logger(),
		leader.EventLoop(),
		leader.RuntimeCfg(),
		leader.BlockChain(),
		leader.Authority(),
		viewStates,
	)

	newViewTriggered := false
	leader.EventLoop().RegisterHandler(hotstuff.NewViewMsg{}, func(_ any) {
		newViewTriggered = true
	})

	block := testutil.CreateBlock(t, leader.Authority())
	leader.BlockChain().Store(block)

	for _, signer := range signers {
		pc := testutil.CreatePC(t, block, signer.Authority())
		vote := hotstuff.VoteMsg{
			ID:          signer.RuntimeCfg().ID(),
			PartialCert: pc,
		}
		votingMachine.CollectVote(vote)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	leader.EventLoop().Run(ctx)

	if !newViewTriggered {
		t.Fatal("expected advancing the view on quorum")
	}
}
