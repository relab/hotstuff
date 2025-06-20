package synchronizer

import (
	"testing"

	"cuelang.org/go/pkg/time"
	"github.com/relab/hotstuff"

	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/protocol"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/disagg/clique"
	"github.com/relab/hotstuff/protocol/leaderrotation/fixedleader"
	"github.com/relab/hotstuff/protocol/rules/chainedhotstuff"
	"github.com/relab/hotstuff/protocol/synchronizer/viewduration"
	"github.com/relab/hotstuff/protocol/votingmachine"
	"github.com/relab/hotstuff/security/cert"
	"github.com/relab/hotstuff/security/crypto/ecdsa"
	"github.com/relab/hotstuff/wiring"
)

const cryptoName = ecdsa.ModuleName

func wireUpSynchronizer(
	t *testing.T,
	essentials *testutil.Essentials,
	commandCache *clientpb.CommandCache,
	viewStates *protocol.ViewStates,
) *Synchronizer {
	t.Helper()
	leaderRotation := fixedleader.New(1)
	consensusRules := chainedhotstuff.New(
		essentials.Logger(),
		essentials.BlockChain(),
	)
	committer := consensus.NewCommitter(
		essentials.EventLoop(),
		essentials.Logger(),
		essentials.BlockChain(),
		viewStates,
		consensusRules,
	)
	votingMachine := votingmachine.New(
		essentials.Logger(),
		essentials.EventLoop(),
		essentials.RuntimeCfg(),
		essentials.BlockChain(),
		essentials.Authority(),
		viewStates,
	)
	depsConsensus := wiring.NewConsensus(
		essentials.EventLoop(),
		essentials.RuntimeCfg(),
		essentials.BlockChain(),
		essentials.Authority(),
		commandCache,
		committer,
		consensusRules,
		leaderRotation,
		clique.New(
			essentials.RuntimeCfg(),
			votingMachine,
			leaderRotation,
			essentials.MockSender(),
		),
	)
	synchronizer := New(
		essentials.EventLoop(),
		essentials.Logger(),
		essentials.RuntimeCfg(),
		essentials.Authority(),
		leaderRotation,
		viewduration.NewFixed(1000*time.Nanosecond),
		depsConsensus.Proposer(),
		depsConsensus.Voter(),
		viewStates,
		essentials.MockSender(),
	)
	return synchronizer
}

func TestAdvanceViewQC(t *testing.T) {
	essentials := testutil.WireUpEssentials(t, 1, ecdsa.ModuleName)
	viewStates, err := protocol.NewViewStates(
		essentials.BlockChain(),
		essentials.Authority(),
	)
	if err != nil {
		t.Fatal(err)
	}
	commandCache := clientpb.NewCommandCache(1)
	synchronizer := wireUpSynchronizer(t, essentials, commandCache, viewStates)

	blockchain := essentials.BlockChain()
	block := hotstuff.NewBlock(
		hotstuff.GetGenesis().Hash(),
		hotstuff.NewQuorumCert(nil, 0, hotstuff.GetGenesis().Hash()),
		&clientpb.Batch{
			Commands: []*clientpb.Command{
				{
					Data: []byte("foo"),
				},
			},
		},
		1,
		1,
	)
	blockchain.Store(block)
	signers := make([]*cert.Authority, 0)
	signers = append(signers, essentials.Authority())
	for i := range 3 {
		id := hotstuff.ID(i + 2)
		replica := testutil.WireUpEssentials(t, id, ecdsa.ModuleName)
		essentials.RuntimeCfg().AddReplica(&hotstuff.ReplicaInfo{
			ID:     id,
			PubKey: replica.RuntimeCfg().PrivateKey().Public(),
		})
		signers = append(signers, replica.Authority())
	}

	qc := testutil.CreateQC(t, block, signers)
	proposer := synchronizer.proposer // TODO(AlanRostem): not very clean, refactor
	commandCache.Add(&clientpb.Command{
		ClientID:       1,
		SequenceNumber: 1,
		Data:           []byte("bar"),
	})
	proposal, err := proposer.CreateProposal(1, viewStates.HighQC(), viewStates.SyncInfo())
	if err != nil {
		t.Fatal(err)
	}
	if err := proposer.Propose(&proposal); err != nil {
		t.Fatal(err)
	}

	synchronizer.advanceView(hotstuff.NewSyncInfo().WithQC(qc))

	if viewStates.View() != 2 {
		t.Errorf("wrong view: expected: %d, got: %d", 2, viewStates.View())
	}
}

func TestAdvanceViewTC(t *testing.T) {
	essentials := testutil.WireUpEssentials(t, 1, ecdsa.ModuleName)
	essentials.MockSender().AddBlockChain(essentials.BlockChain())
	viewStates, err := protocol.NewViewStates(
		essentials.BlockChain(),
		essentials.Authority(),
	)
	if err != nil {
		t.Fatal(err)
	}
	commandCache := clientpb.NewCommandCache(1)
	synchronizer := wireUpSynchronizer(t, essentials, commandCache, viewStates)

	signers := make([]*cert.Authority, 0)
	signers = append(signers, essentials.Authority())
	for i := range 3 {
		id := hotstuff.ID(i + 2)
		replica := testutil.WireUpEssentials(t, id, ecdsa.ModuleName)
		essentials.RuntimeCfg().AddReplica(&hotstuff.ReplicaInfo{
			ID:     id,
			PubKey: replica.RuntimeCfg().PrivateKey().Public(),
		})
		signers = append(signers, replica.Authority())
	}

	tc := testutil.CreateTC(t, 1, signers)

	proposer := synchronizer.proposer // TODO(AlanRostem): not very clean, refactor
	for i := range 2 {
		// adding multiple commands so the next call CreateProposal
		// in advanceView doesn't block
		commandCache.Add(&clientpb.Command{
			ClientID:       1,
			SequenceNumber: uint64(i + 1),
			Data:           []byte("bar"),
		})
	}
	proposal, err := proposer.CreateProposal(1, viewStates.HighQC(), viewStates.SyncInfo())
	if err != nil {
		t.Fatal(err)
	}
	if err := proposer.Propose(&proposal); err != nil {
		t.Fatal(err)
	}
	synchronizer.advanceView(hotstuff.NewSyncInfo().WithTC(tc))

	if viewStates.View() != 2 {
		t.Errorf("wrong view: expected: %d, got: %d", 2, viewStates.View())
	}
}
