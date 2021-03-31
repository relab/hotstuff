package chainedhotstuff

import (
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/relab/hotstuff"
	ecdsacrypto "github.com/relab/hotstuff/crypto/ecdsa"
	"github.com/relab/hotstuff/internal/mocks"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/leaderrotation"
	"github.com/relab/hotstuff/synchronizer"
)

// TestPropose checks that a leader broadcasts a new proposal, and then sends a vote to the next leader
func TestPropose(t *testing.T) {
	// Setup mocks
	ctrl := gomock.NewController(t)
	hs := New()
	builder := testutil.TestModules(t, ctrl, 1, testutil.GenerateKey(t))
	synchronizer := synchronizer.New(testutil.FixedTimeout(time.Second))
	cfg, replicas := testutil.CreateMockConfigWithReplicas(t, ctrl, 2)
	builder.Register(hs, cfg, testutil.NewLeaderRotation(t, 1, 2), synchronizer)
	builder.Build()

	// RULES:

	// leader should propose to other replicas.
	cfg.EXPECT().Propose(gomock.AssignableToTypeOf(&hotstuff.Block{}))

	// leader should send its own vote to the next leader.
	replicas[1].EXPECT().Vote(gomock.Any())

	hs.Propose()

	if hs.LastVote() != 1 {
		t.Errorf("Wrong view: got: %d, want: %d", hs.LastVote(), 1)
	}
}

// TestCommit checks that a replica commits and executes a valid branch
func TestCommit(t *testing.T) {
	const n = 4
	ctrl := gomock.NewController(t)
	hs := New()
	keys := testutil.GenerateKeys(t, n)
	bl := testutil.CreateBuilders(t, ctrl, n, keys...)
	acceptor := mocks.NewMockAcceptor(ctrl)
	executor := mocks.NewMockExecutor(ctrl)
	synchronizer := synchronizer.New(testutil.FixedTimeout(time.Second))
	cfg, replicas := testutil.CreateMockConfigWithReplicas(t, ctrl, n, keys...)
	bl[0].Register(hs, cfg, acceptor, executor, synchronizer, leaderrotation.NewFixed(2))
	hl := bl.Build()
	signers := hl.Signers()

	// create the needed blocks and QCs
	genesisQC := ecdsacrypto.NewQuorumCert(map[hotstuff.ID]*ecdsacrypto.Signature{}, hotstuff.GetGenesis().Hash())
	b1 := testutil.NewProposeMsg(hotstuff.GetGenesis().Hash(), genesisQC, "1", 1, 2)
	b1QC := testutil.CreateQC(t, b1.Block, signers)
	b2 := testutil.NewProposeMsg(b1.Block.Hash(), b1QC, "2", 2, 2)
	b2QC := testutil.CreateQC(t, b2.Block, signers)
	b3 := testutil.NewProposeMsg(b2.Block.Hash(), b2QC, "3", 3, 2)
	b3QC := testutil.CreateQC(t, b3.Block, signers)
	b4 := testutil.NewProposeMsg(b3.Block.Hash(), b3QC, "4", 4, 2)

	// the second replica will be the leader, so we expect it to receive votes
	replicas[1].EXPECT().Vote(gomock.Any()).AnyTimes()
	replicas[1].EXPECT().NewView(gomock.Any()).AnyTimes()

	// executor will check that the correct command is executed
	executor.EXPECT().Exec(gomock.Any()).Do(func(arg interface{}) {
		if arg.(hotstuff.Command) != b1.Block.Command() {
			t.Errorf("Wrong command executed: got: %s, want: %s", arg, b1.Block.Command())
		}
	})

	// acceptor expects to receive the commands in order
	gomock.InOrder(
		acceptor.EXPECT().Accept(hotstuff.Command("1")).Return(true),
		acceptor.EXPECT().Accept(hotstuff.Command("2")).Return(true),
		acceptor.EXPECT().Accept(hotstuff.Command("3")).Return(true),
		acceptor.EXPECT().Accept(hotstuff.Command("4")).Return(true),
	)

	hs.OnPropose(b1)
	hs.OnPropose(b2)
	hs.OnPropose(b3)
	hs.OnPropose(b4)
}

// TestVote checks that a leader can collect votes on a proposal to form a QC
func TestVote(t *testing.T) {
	const n = 4
	ctrl := gomock.NewController(t)

	hs := New()
	bl := testutil.CreateBuilders(t, ctrl, n)
	synchronizer := synchronizer.New(testutil.FixedTimeout(time.Second))
	bl[0].Register(hs, synchronizer)
	hl := bl.Build()

	// expect that the replica will propose after receiving enough votes.
	hl[0].Config().(*mocks.MockConfig).EXPECT().Propose(gomock.AssignableToTypeOf(hotstuff.GetGenesis()))

	b := testutil.NewProposeMsg(hotstuff.GetGenesis().Hash(), hl[0].ViewSynchronizer().HighQC(), "test", 1, 1)

	hs.OnPropose(b)

	for i, signer := range hl.Signers()[1:] {
		pc, err := signer.CreatePartialCert(b.Block)
		if err != nil {
			t.Fatalf("Failed to create partial certificate: %v", err)
		}
		hs.OnVote(hotstuff.VoteMsg{ID: hotstuff.ID(i + 1), PartialCert: pc})
	}

	if hl[0].ViewSynchronizer().HighQC().BlockHash() != b.Block.Hash() {
		t.Errorf("HighQC was not updated.")
	}
}

// TestForkingAttack shows that it is possible to execute a forking attack against HotStuff.
// A forking attack is when a proposal creates a fork in the block chain, leading to some commands never being executed.
// Such as scenario is illustrated in the diagram below.
// Let the arrows from the sides of the blocks represent parent links,
// while the arrows from the corners of the blocks represent QC links:
//          __________________________________
//         /                                  \
//        /                                    +---+
//       /       +-----------------------------| E |
//      / ___    |  ___       ___              +---+
//     / /   \   v /   \     /   \
//  +---+     +---+     +---+     +---+
//  | A |<----| B |<----| C |<----| D |
//  +---+     +---+     +---+     +---+
//
// Here, block E creates a new fork which means that blocks C and D will not be executed.
func TestForkingAttack(t *testing.T) {
	const n = 4
	ctrl := gomock.NewController(t)
	hs := New()
	keys := testutil.GenerateKeys(t, n)
	bl := testutil.CreateBuilders(t, ctrl, n, keys...)
	cfg, replicas := testutil.CreateMockConfigWithReplicas(t, ctrl, n, keys...)
	executor := mocks.NewMockExecutor(ctrl)
	synchronizer := synchronizer.New(testutil.FixedTimeout(time.Second))
	bl[0].Register(hs, cfg, executor, synchronizer, leaderrotation.NewFixed(2))
	hl := bl.Build()
	signers := hl.Signers()

	// configure mocks
	replicas[1].EXPECT().Vote(gomock.Any()).AnyTimes()
	replicas[1].EXPECT().NewView(gomock.Any()).AnyTimes()

	genesisQC := ecdsacrypto.NewQuorumCert(make(map[hotstuff.ID]*ecdsacrypto.Signature), hotstuff.GetGenesis().Hash())
	a := testutil.NewProposeMsg(hotstuff.GetGenesis().Hash(), genesisQC, "A", 1, 2)
	aQC := testutil.CreateQC(t, a.Block, signers)
	b := testutil.NewProposeMsg(a.Block.Hash(), aQC, "B", 2, 2)
	bQC := testutil.CreateQC(t, b.Block, signers)
	c := testutil.NewProposeMsg(b.Block.Hash(), bQC, "C", 3, 2)
	cQC := testutil.CreateQC(t, c.Block, signers)
	d := testutil.NewProposeMsg(c.Block.Hash(), cQC, "D", 4, 2)
	e := testutil.NewProposeMsg(b.Block.Hash(), aQC, "E", 5, 2)

	// expected order of execution
	gomock.InOrder(
		executor.EXPECT().Exec(a.Block.Command()),
		executor.EXPECT().Exec(b.Block.Command()),
		executor.EXPECT().Exec(e.Block.Command()),
	)

	hs.OnPropose(a)
	hs.OnPropose(b)
	hs.OnPropose(c)
	hs.OnPropose(d)

	// sanity check
	if hs.(*chainedhotstuff).bLock != b.Block {
		t.Fatalf("Not locked on B!")
	}

	hs.OnPropose(e)

	// advance views until E is executed
	block := advanceView(t, hs, e.Block, signers)
	block = advanceView(t, hs, block, signers)
	_ = advanceView(t, hs, block, signers)
}

func advanceView(t *testing.T, hs hotstuff.Consensus, lastProposal *hotstuff.Block, signers []hotstuff.Signer) *hotstuff.Block {
	t.Helper()

	qc := testutil.CreateQC(t, lastProposal, signers)
	b := hotstuff.NewBlock(lastProposal.Hash(), qc, "foo", hs.LastVote()+1, 2)
	hs.OnPropose(hotstuff.ProposeMsg{ID: b.Proposer(), Block: b})
	return b
}
