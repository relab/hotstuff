package chainedhotstuff

import (
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
	return &ecdsacrypto.PrivateKey{pk}
}

// TestPropose checks that a leader broadcasts a new proposal, and then sends a vote to the next leader
func TestPropose(t *testing.T) {
	pk := createKey(t)

	// Setup mocks
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// this replica
	replica1 := testutil.CreateMockReplica(t, ctrl, hotstuff.ID(1), nil)

	// the next leader
	replica2 := testutil.CreateMockReplica(t, ctrl, hotstuff.ID(2), nil)

	// basic configuration with the two replicas
	cfg := testutil.CreateMockConfig(t, ctrl, hotstuff.ID(1), pk)
	cfg.EXPECT().QuorumSize().AnyTimes().Return(3)
	testutil.ConfigAddReplica(t, cfg, replica1)
	testutil.ConfigAddReplica(t, cfg, replica2)

	// command queue that returns "foo"
	queue := mocks.NewMockCommandQueue(ctrl)
	queue.EXPECT().GetCommand().DoAndReturn(func() *hotstuff.Command {
		cmd := hotstuff.Command("foo")
		return &cmd
	})

	// we should not actually need to execute anything yet
	exec := mocks.NewMockExecutor(ctrl)

	// dumb acceptor that only returns true
	acceptor := mocks.NewMockAcceptor(ctrl)
	acceptor.EXPECT().Accept(gomock.Any()).Return(true)

	synchronizer := mocks.NewMockViewSynchronizer(ctrl)
	// the following should be called once
	synchronizer.EXPECT().Init(gomock.Any())
	synchronizer.EXPECT().OnPropose()

	// RULES:

	// leader should propose to other replicas.
	cfg.EXPECT().Propose(gomock.Any())

	// leader should send its own vote to the next leader.
	replica2.EXPECT().Vote(gomock.Any())

	// leader should ask synchronizer for the id of the next leader
	synchronizer.EXPECT().GetLeader(hotstuff.View(2)).Return(hotstuff.ID(2))

	hs := Builder{
		Config:       cfg,
		CommandQueue: queue,
		Acceptor:     acceptor,
		Executor:     exec,
		Synchronizer: synchronizer,
	}.Build()

	hs.Propose()

	if hs.LastVote() != 1 {
		t.Errorf("Wrong view: got: %d, want: %d", hs.LastVote(), 1)
	}
}

func createQC(t *testing.T, block *hotstuff.Block, signers []hotstuff.Signer) hotstuff.QuorumCert {
	t.Helper()
	if len(signers) == 0 {
		return nil
	}
	pcs := make([]hotstuff.PartialCert, 0, len(signers))
	for _, signer := range signers {
		pc, err := signer.Sign(block)
		if err != nil {
			t.Fatalf("Failed to sign block: %v", err)
		}
		pcs = append(pcs, pc)
	}
	qc, err := signers[0].CreateQuorumCert(block, pcs)
	if err != nil {
		t.Fatalf("Faield to create QC: %v", err)
	}
	return qc
}

// TestCommit checks that a replica commits and executes a valid branch
func TestCommit(t *testing.T) {
	// Create 3 signers, and then create at least 3 proposals to submit to the replica.
	// Then check that the replica has executed the first one.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	keys := make([]hotstuff.PrivateKey, 0, 3)
	replicas := make([]*mocks.MockReplica, 0, 3)
	configs := make([]*mocks.MockConfig, 0, 3)
	signers := make([]hotstuff.Signer, 0, 3)

	for i := 0; i < 3; i++ {
		id := hotstuff.ID(i) + 1
		keys = append(keys, createKey(t))
		configs = append(configs, testutil.CreateMockConfig(t, ctrl, id, keys[i]))
		replicas = append(replicas, testutil.CreateMockReplica(t, ctrl, id, keys[i].PublicKey()))
		signer, _ := ecdsacrypto.New(configs[i])
		signers = append(signers, signer)
	}

	for _, config := range configs {
		for _, replica := range replicas {
			testutil.ConfigAddReplica(t, config, replica)
		}
	}

	// create the needed blocks and QCs
	genesisQC := ecdsacrypto.NewQuorumCert(map[hotstuff.ID]*ecdsacrypto.Signature{}, hotstuff.GetGenesis().Hash())
	b1 := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), genesisQC, "1", 1, 1)
	b1QC := createQC(t, b1, signers)
	b2 := hotstuff.NewBlock(b1.Hash(), b1QC, "2", 2, 1)
	b2QC := createQC(t, b2, signers)
	b3 := hotstuff.NewBlock(b2.Hash(), b2QC, "3", 3, 1)
	b3QC := createQC(t, b3, signers)
	b4 := hotstuff.NewBlock(b3.Hash(), b3QC, "4", 4, 1)

	// set up mocks for hotstuff
	pk := createKey(t)

	cfg := testutil.CreateMockConfig(t, ctrl, hotstuff.ID(4), pk)
	cfg.EXPECT().QuorumSize().AnyTimes().Return(3)

	for _, replica := range replicas {
		testutil.ConfigAddReplica(t, cfg, replica)
	}

	// the first replica will be the leader, so we expect it to receive votes
	replicas[0].EXPECT().Vote(gomock.Any()).AnyTimes()

	// command queue should not be used
	queue := mocks.NewMockCommandQueue(ctrl)

	// executor will check that the correct command is executed
	exec := mocks.NewMockExecutor(ctrl)
	exec.EXPECT().Exec(gomock.Any()).Do(func(arg interface{}) {
		if arg.(hotstuff.Command) != b1.Command() {
			t.Errorf("Wrong command executed: got: %s, want: %s", arg, b1.Command())
		}
	})

	// acceptor expects to receive the commands in order
	acceptor := mocks.NewMockAcceptor(ctrl)
	gomock.InOrder(
		acceptor.EXPECT().Accept(hotstuff.Command("1")).Return(true),
		acceptor.EXPECT().Accept(hotstuff.Command("2")).Return(true),
		acceptor.EXPECT().Accept(hotstuff.Command("3")).Return(true),
		acceptor.EXPECT().Accept(hotstuff.Command("4")).Return(true),
	)

	synchronizer := mocks.NewMockViewSynchronizer(ctrl)
	synchronizer.EXPECT().GetLeader(gomock.Any()).AnyTimes().Return(hotstuff.ID(1))
	synchronizer.EXPECT().OnPropose().AnyTimes()
	// the following should be called once
	synchronizer.EXPECT().Init(gomock.Any())

	hs := Builder{
		Config:       cfg,
		CommandQueue: queue,
		Acceptor:     acceptor,
		Executor:     exec,
		Synchronizer: synchronizer,
	}.Build()

	hs.OnPropose(b1)
	hs.OnPropose(b2)
	hs.OnPropose(b3)
	hs.OnPropose(b4)
}

// TestVote checks that a leader can collect votes on a proposal to form a QC
func TestVote(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	keys := make([]hotstuff.PrivateKey, 0, 3)
	replicas := make([]*mocks.MockReplica, 0, 3)
	configs := make([]*mocks.MockConfig, 0, 3)
	signers := make([]hotstuff.Signer, 0, 3)

	for i := 0; i < 3; i++ {
		id := hotstuff.ID(i) + 1
		keys = append(keys, createKey(t))
		configs = append(configs, testutil.CreateMockConfig(t, ctrl, id, keys[i]))
		replicas = append(replicas, testutil.CreateMockReplica(t, ctrl, id, keys[i].PublicKey()))
		signer, _ := ecdsacrypto.New(configs[i])
		signers = append(signers, signer)
	}

	for _, config := range configs {
		for _, replica := range replicas {
			testutil.ConfigAddReplica(t, config, replica)
		}
	}

	// set up mocks for hotstuff
	pk := createKey(t)

	replica := testutil.CreateMockReplica(t, ctrl, 4, pk.PublicKey())
	cfg := testutil.CreateMockConfig(t, ctrl, hotstuff.ID(4), pk)
	cfg.EXPECT().QuorumSize().AnyTimes().Return(3)
	testutil.ConfigAddReplica(t, cfg, replica)

	for _, replica := range replicas {
		testutil.ConfigAddReplica(t, cfg, replica)
	}

	// command queue should not be used
	queue := mocks.NewMockCommandQueue(ctrl)

	// executor should not be used
	exec := mocks.NewMockExecutor(ctrl)

	// acceptor should accept one block
	acceptor := mocks.NewMockAcceptor(ctrl)
	acceptor.EXPECT().Accept(gomock.Any()).Return(true)

	// synchronizer should expect one proposal and one finishQC
	synchronizer := mocks.NewMockViewSynchronizer(ctrl)
	synchronizer.EXPECT().Init(gomock.Any())
	synchronizer.EXPECT().OnPropose()
	synchronizer.EXPECT().OnFinishQC()
	synchronizer.EXPECT().GetLeader(gomock.Any()).AnyTimes().Return(hotstuff.ID(4))

	hs := Builder{
		Config:       cfg,
		CommandQueue: queue,
		Acceptor:     acceptor,
		Executor:     exec,
		Synchronizer: synchronizer,
	}.Build()

	b := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), hs.HighQC(), "test", 1, 1)

	hs.OnPropose(b)

	for _, signer := range signers {
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
