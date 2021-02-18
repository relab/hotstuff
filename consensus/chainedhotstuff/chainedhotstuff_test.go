package chainedhotstuff

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/crypto"
	ecdsacrypto "github.com/relab/hotstuff/crypto/ecdsa"
	"github.com/relab/hotstuff/internal/mocks"
	"github.com/relab/hotstuff/internal/testutil"
)

func createKey(t *testing.T) *ecdsacrypto.PrivateKey {
	t.Helper()
	pk, err := crypto.GeneratePrivateKey()
	if err != nil {
		t.Errorf("Failed to generate private key: %v", err)
	}
	return &ecdsacrypto.PrivateKey{PrivateKey: pk}
}

// TestPropose checks that a leader broadcasts a new proposal, and then sends a vote to the next leader
func TestPropose(t *testing.T) {
	// Setup mocks
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	td := newTestData(t, ctrl, 4)

	// command queue that returns "foo"
	td.commands.EXPECT().GetCommand().DoAndReturn(func() *hotstuff.Command {
		cmd := hotstuff.Command("foo")
		return &cmd
	})

	td.acceptor.EXPECT().Accept(gomock.Any()).Return(true)

	td.synchronizer.EXPECT().OnPropose()

	// RULES:

	// leader should propose to other replicas.
	td.config.EXPECT().Propose(gomock.Any())

	// leader should send its own vote to the next leader.
	td.replicas[0].EXPECT().Vote(gomock.Any())

	// leader should ask synchronizer for the id of the next leader
	td.synchronizer.EXPECT().GetLeader(hotstuff.View(2)).Return(hotstuff.ID(1))

	hs := td.build()

	hs.Propose()

	if hs.LastVote() != 1 {
		t.Errorf("Wrong view: got: %d, want: %d", hs.LastVote(), 1)
	}
}

type testData struct {
	t        *testing.T
	replicas []*mocks.MockReplica
	configs  []*mocks.MockConfig
	signers  []hotstuff.Signer

	acceptor     *mocks.MockAcceptor
	commands     *mocks.MockCommandQueue
	config       *mocks.MockConfig
	executor     *mocks.MockExecutor
	synchronizer *mocks.MockViewSynchronizer
}

func (td testData) build() hotstuff.Consensus {
	td.t.Helper()

	return Builder{
		Acceptor:     td.acceptor,
		Config:       td.config,
		CommandQueue: td.commands,
		Executor:     td.executor,
		Synchronizer: td.synchronizer,
	}.Build()
}

func (td testData) createQC(block *hotstuff.Block) hotstuff.QuorumCert {
	td.t.Helper()

	return testutil.CreateQC(td.t, block, td.signers)
}

func (td testData) advanceView(hs hotstuff.Consensus, lastProposal *hotstuff.Block) *hotstuff.Block {
	td.t.Helper()

	qc := td.createQC(lastProposal)
	b := hotstuff.NewBlock(lastProposal.Hash(), qc, "foo", hs.LastVote()+1, 1)
	hs.OnPropose(b)
	return b
}

func newTestData(t *testing.T, ctrl *gomock.Controller, n int) testData {
	t.Helper()

	td := testData{
		t:        t,
		replicas: make([]*mocks.MockReplica, n-1),
		configs:  make([]*mocks.MockConfig, n-1),
		signers:  make([]hotstuff.Signer, n-1),
	}

	for i := 0; i < n-1; i++ {
		id := hotstuff.ID(i) + 1
		key := createKey(t)
		td.configs[i] = testutil.CreateMockConfig(t, ctrl, id, key)
		td.replicas[i] = testutil.CreateMockReplica(t, ctrl, id, key.PublicKey())
		signer, _ := ecdsacrypto.New(td.configs[i])
		td.signers[i] = signer
	}

	for _, config := range td.configs {
		for _, replica := range td.replicas {
			testutil.ConfigAddReplica(t, config, replica)
		}
	}

	pk := createKey(t)

	td.acceptor = mocks.NewMockAcceptor(ctrl)
	td.commands = mocks.NewMockCommandQueue(ctrl)
	td.config = testutil.CreateMockConfig(t, ctrl, hotstuff.ID(n), pk)
	td.executor = mocks.NewMockExecutor(ctrl)
	td.synchronizer = mocks.NewMockViewSynchronizer(ctrl)

	// basic configuration
	td.config.EXPECT().QuorumSize().AnyTimes().Return(n - (n-1)/3)
	td.synchronizer.EXPECT().Init(gomock.Any())

	r := testutil.CreateMockReplica(t, ctrl, hotstuff.ID(n), pk.PublicKey())
	testutil.ConfigAddReplica(t, td.config, r)

	for _, replica := range td.replicas {
		testutil.ConfigAddReplica(t, td.config, replica)
	}

	return td
}

// TestCommit checks that a replica commits and executes a valid branch
func TestCommit(t *testing.T) {
	// Create 3 signers, and then create at least 3 proposals to submit to the replica.
	// Then check that the replica has executed the first one.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	td := newTestData(t, ctrl, 4)

	// create the needed blocks and QCs
	genesisQC := ecdsacrypto.NewQuorumCert(map[hotstuff.ID]*ecdsacrypto.Signature{}, hotstuff.GetGenesis().Hash())
	b1 := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), genesisQC, "1", 1, 1)
	b1QC := testutil.CreateQC(t, b1, td.signers)
	b2 := hotstuff.NewBlock(b1.Hash(), b1QC, "2", 2, 1)
	b2QC := testutil.CreateQC(t, b2, td.signers)
	b3 := hotstuff.NewBlock(b2.Hash(), b2QC, "3", 3, 1)
	b3QC := testutil.CreateQC(t, b3, td.signers)
	b4 := hotstuff.NewBlock(b3.Hash(), b3QC, "4", 4, 1)

	// the first replica will be the leader, so we expect it to receive votes
	td.replicas[0].EXPECT().Vote(gomock.Any()).AnyTimes()

	// executor will check that the correct command is executed
	td.executor.EXPECT().Exec(gomock.Any()).Do(func(arg interface{}) {
		if arg.(hotstuff.Command) != b1.Command() {
			t.Errorf("Wrong command executed: got: %s, want: %s", arg, b1.Command())
		}
	})

	// acceptor expects to receive the commands in order
	gomock.InOrder(
		td.acceptor.EXPECT().Accept(hotstuff.Command("1")).Return(true),
		td.acceptor.EXPECT().Accept(hotstuff.Command("2")).Return(true),
		td.acceptor.EXPECT().Accept(hotstuff.Command("3")).Return(true),
		td.acceptor.EXPECT().Accept(hotstuff.Command("4")).Return(true),
	)

	td.synchronizer.EXPECT().GetLeader(gomock.Any()).AnyTimes().Return(hotstuff.ID(1))
	td.synchronizer.EXPECT().OnPropose().AnyTimes()

	hs := td.build()

	hs.OnPropose(b1)
	hs.OnPropose(b2)
	hs.OnPropose(b3)
	hs.OnPropose(b4)
}

// TestVote checks that a leader can collect votes on a proposal to form a QC
func TestVote(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	td := newTestData(t, ctrl, 4)

	td.acceptor.EXPECT().Accept(gomock.Any()).Return(true)

	// synchronizer should expect one proposal and one finishQC
	td.synchronizer.EXPECT().OnPropose()
	td.synchronizer.EXPECT().OnFinishQC()
	td.synchronizer.EXPECT().GetLeader(gomock.Any()).AnyTimes().Return(hotstuff.ID(4))

	hs := td.build()

	b := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), hs.HighQC(), "test", 1, 1)

	hs.OnPropose(b)

	for _, signer := range td.signers {
		pc, err := signer.Sign(b)
		if err != nil {
			t.Fatalf("Failed to create partial certificate: %v", err)
		}
		hs.OnVote(pc)
	}

	if hs.HighQC().BlockHash() != b.Hash() {
		t.Errorf("HighQC was not updated.")
	}
}

// TestFetchBlock checks that a replica can fetch a block in order to create a QC
func TestFetchBlock(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	td := newTestData(t, ctrl, 4)

	// create test data
	votesSent := make(chan struct{})
	qcCreated := make(chan struct{})
	genesisQC := ecdsacrypto.NewQuorumCert(map[hotstuff.ID]*ecdsacrypto.Signature{}, hotstuff.GetGenesis().Hash())
	b := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), genesisQC, "foo", 1, 1)
	votes := testutil.CreatePCs(t, b, td.signers)

	hs := td.build()

	// configure mocks
	td.config.
		EXPECT().
		Fetch(gomock.Any(), gomock.AssignableToTypeOf(b.Hash())).
		Do(func(_ context.Context, h hotstuff.Hash) {
			// wait for all votes to be sent
			go func() {
				<-votesSent
				hs.OnDeliver(b)
			}()
		})

	td.synchronizer.EXPECT().OnFinishQC().Do(func() {
		close(qcCreated)
	})

	for _, vote := range votes {
		hs.OnVote(vote)
	}

	close(votesSent)
	<-qcCreated

	if hs.HighQC().BlockHash() != b.Hash() {
		t.Fatalf("A new QC was not created.")
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
	ctrl := gomock.NewController(t)
	td := newTestData(t, ctrl, 4)
	// configure mocks
	td.replicas[0].EXPECT().Vote(gomock.Any()).AnyTimes()
	td.synchronizer.EXPECT().OnPropose().AnyTimes()
	td.synchronizer.EXPECT().GetLeader(gomock.Any()).AnyTimes().Return(hotstuff.ID(1))
	td.acceptor.EXPECT().Accept(gomock.Any()).AnyTimes().Return(true)

	hs := td.build()

	genesisQC := ecdsacrypto.NewQuorumCert(make(map[hotstuff.ID]*ecdsacrypto.Signature), hotstuff.GetGenesis().Hash())
	a := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), genesisQC, "A", 1, 1)
	aQC := td.createQC(a)
	b := hotstuff.NewBlock(a.Hash(), aQC, "B", 2, 1)
	bQC := td.createQC(b)
	c := hotstuff.NewBlock(b.Hash(), bQC, "C", 3, 1)
	cQC := td.createQC(c)
	d := hotstuff.NewBlock(c.Hash(), cQC, "D", 4, 1)
	e := hotstuff.NewBlock(b.Hash(), aQC, "E", 5, 1)

	// expected order of execution
	gomock.InOrder(
		td.executor.EXPECT().Exec(a.Command()),
		td.executor.EXPECT().Exec(b.Command()),
		td.executor.EXPECT().Exec(e.Command()),
	)

	hs.OnPropose(a)
	hs.OnPropose(b)
	hs.OnPropose(c)
	hs.OnPropose(d)

	// sanity check
	if hs.(*chainedhotstuff).bLock != b {
		t.Fatalf("Not locked on B!")
	}

	hs.OnPropose(e)

	// advance views until E is executed
	block := td.advanceView(hs, e)
	block = td.advanceView(hs, block)
	_ = td.advanceView(hs, block)
}
