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

	if hs.View() != 1 {
		t.Errorf("Wrong view: got: %d, want: %d", hs.View(), 1)
	}
}
