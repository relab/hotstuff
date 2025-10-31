package votingmachine_test

import (
	"context"
	"testing"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/protocol"
	"github.com/relab/hotstuff/protocol/votingmachine"
	"github.com/relab/hotstuff/security/crypto"
)

func TestCollectVote(t *testing.T) {
	signers := testutil.NewEssentialsSet(t, 4, crypto.NameECDSA)
	leader := signers[0]
	viewStates, err := protocol.NewViewStates(
		leader.Blockchain(),
		leader.Authority(),
	)
	if err != nil {
		t.Fatal(err)
	}
	votingMachine := votingmachine.New(
		leader.Logger(),
		leader.EventLoop(),
		leader.RuntimeCfg(),
		leader.Blockchain(),
		leader.Authority(),
		viewStates,
	)

	newViewTriggered := false
	leader.EventLoop().RegisterHandler(hotstuff.NewViewMsg{}, func(_ any) {
		newViewTriggered = true
	})

	block := testutil.CreateBlock(t, leader.Authority())
	leader.Blockchain().Store(block)

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

func TestCollectVoteWithDuplicates(t *testing.T) {
	signers := testutil.NewEssentialsSet(t, 4, crypto.NameECDSA)
	leader := signers[0]
	viewStates, err := protocol.NewViewStates(
		leader.Blockchain(),
		leader.Authority(),
	)
	if err != nil {
		t.Fatal(err)
	}
	votingMachine := votingmachine.New(
		leader.Logger(),
		leader.EventLoop(),
		leader.RuntimeCfg(),
		leader.Blockchain(),
		leader.Authority(),
		viewStates,
	)

	newViewTriggered := false
	leader.EventLoop().RegisterHandler(hotstuff.NewViewMsg{}, func(_ any) {
		newViewTriggered = true
	})

	block := testutil.CreateBlock(t, leader.Authority())
	leader.Blockchain().Store(block)

	// Send duplicate votes from the first signer
	// This will cause an error unless the duplicate is filtered
	signer := signers[0]
	pc := testutil.CreatePC(t, block, signer.Authority())
	vote := hotstuff.VoteMsg{
		ID:          signer.RuntimeCfg().ID(),
		PartialCert: pc,
	}
	votingMachine.CollectVote(vote)

	// Collect votes from all signers
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
		t.Fatal("expected advancing the view on quorum even with duplicate votes")
	}
}
