// Package testutil provides helper methods that are useful for implementing tests.
package testutil

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/blockchain"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/crypto"
	"github.com/relab/hotstuff/crypto/bls12"
	"github.com/relab/hotstuff/crypto/ecdsa"
	"github.com/relab/hotstuff/crypto/keygen"
	"github.com/relab/hotstuff/internal/mocks"
	"github.com/relab/hotstuff/leaderrotation"
	"github.com/relab/hotstuff/logging"
	"github.com/relab/hotstuff/synchronizer"
)

// TestModules returns a builder containing default modules for testing.
func TestModules(t *testing.T, ctrl *gomock.Controller, id hotstuff.ID, privkey consensus.PrivateKey) consensus.Builder {
	t.Helper()
	builder := consensus.NewBuilder(id, privkey)

	acceptor := mocks.NewMockAcceptor(ctrl)
	acceptor.EXPECT().Accept(gomock.AssignableToTypeOf(consensus.Command(""))).AnyTimes().Return(true)
	acceptor.EXPECT().Proposed(gomock.Any()).AnyTimes()

	executor := mocks.NewMockExecutor(ctrl)
	executor.EXPECT().Exec(gomock.AssignableToTypeOf(consensus.Command(""))).AnyTimes()

	commandQ := mocks.NewMockCommandQueue(ctrl)
	commandQ.EXPECT().Get(gomock.Any()).AnyTimes().Return(consensus.Command("foo"), true)

	signer := crypto.NewCache(ecdsa.New(), 10)

	config := mocks.NewMockConfiguration(ctrl)
	config.EXPECT().Len().AnyTimes().Return(1)
	config.EXPECT().QuorumSize().AnyTimes().Return(3)

	replica := CreateMockReplica(t, ctrl, id, privkey.Public())
	ConfigAddReplica(t, config, replica)
	config.EXPECT().Replicas().AnyTimes().Return((map[hotstuff.ID]consensus.Replica{1: replica}))

	synchronizer := mocks.NewMockSynchronizer(ctrl)
	synchronizer.EXPECT().Start(gomock.Any()).AnyTimes()
	synchronizer.EXPECT().ViewContext().AnyTimes().Return(context.Background())

	builder.Register(
		logging.New(fmt.Sprintf("hs%d", id)),
		blockchain.New(),
		mocks.NewMockConsensus(ctrl),
		leaderrotation.NewFixed(1),
		synchronizer,
		config,
		signer,
		acceptor,
		executor,
		commandQ,
	)
	return builder
}

// BuilderList is a helper type to perform actions on a set of builders.
type BuilderList []*consensus.Builder

// HotStuffList is a helper type to perform actions on a set of HotStuff instances.
type HotStuffList []*consensus.Modules

// Build calls Build() for all of the builders.
func (bl BuilderList) Build() HotStuffList {
	hl := HotStuffList{}
	for _, hs := range bl {
		hl = append(hl, hs.Build())
	}
	return hl
}

// Signers returns the set of signers from all of the HotStuff instances.
func (hl HotStuffList) Signers() (signers []consensus.Crypto) {
	signers = make([]consensus.Crypto, len(hl))
	for i, hs := range hl {
		signers[i] = hs.Crypto()
	}
	return signers
}

// Verifiers returns the set of verifiers from all of the HotStuff instances.
func (hl HotStuffList) Verifiers() (verifiers []consensus.Crypto) {
	verifiers = make([]consensus.Crypto, len(hl))
	for i, hs := range hl {
		verifiers[i] = hs.Crypto()
	}
	return verifiers
}

// Keys returns the set of private keys from all of the HotStuff instances.
func (hl HotStuffList) Keys() (keys []consensus.PrivateKey) {
	keys = make([]consensus.PrivateKey, len(hl))
	for i, hs := range hl {
		keys[i] = hs.PrivateKey()
	}
	return keys
}

// CreateBuilders creates n builders with default consensus. Configurations are initialized with replicas.
func CreateBuilders(t *testing.T, ctrl *gomock.Controller, n int, keys ...consensus.PrivateKey) (builders BuilderList) {
	t.Helper()
	builders = make([]*consensus.Builder, n)
	replicas := make([]*mocks.MockReplica, n)
	configs := make([]*mocks.MockConfiguration, n)
	for i := 0; i < n; i++ {
		id := hotstuff.ID(i + 1)
		var key consensus.PrivateKey
		if i < len(keys) {
			key = keys[i]
		} else {
			key = GenerateECDSAKey(t)
		}
		configs[i] = mocks.NewMockConfiguration(ctrl)
		replicas[i] = CreateMockReplica(t, ctrl, id, key.Public())
		builders[i] = new(consensus.Builder)
		*builders[i] = TestModules(t, ctrl, id, key)
		builders[i].Register(configs[i]) // replaces the config registered by TestModules()
	}
	for _, config := range configs {
		for _, replica := range replicas {
			ConfigAddReplica(t, config, replica)
		}
		config.EXPECT().Len().AnyTimes().Return(len(replicas))
		config.EXPECT().QuorumSize().AnyTimes().Return(hotstuff.QuorumSize(len(replicas)))
		config.EXPECT().Replicas().AnyTimes().DoAndReturn(func() map[hotstuff.ID]consensus.Replica {
			m := make(map[hotstuff.ID]consensus.Replica)
			for _, replica := range replicas {
				m[replica.ID()] = replica
			}
			return m
		})
	}
	return builders
}

// CreateMockConfigurationWithReplicas creates a configuration with n replicas.
func CreateMockConfigurationWithReplicas(t *testing.T, ctrl *gomock.Controller, n int, keys ...consensus.PrivateKey) (*mocks.MockConfiguration, []*mocks.MockReplica) {
	t.Helper()
	cfg := mocks.NewMockConfiguration(ctrl)
	replicas := make([]*mocks.MockReplica, n)
	if len(keys) == 0 {
		keys = make([]consensus.PrivateKey, 0, n)
	}
	for i := 0; i < n; i++ {
		if len(keys) <= i {
			keys = append(keys, GenerateECDSAKey(t))
		}
		replicas[i] = CreateMockReplica(t, ctrl, hotstuff.ID(i+1), keys[i].Public())
		ConfigAddReplica(t, cfg, replicas[i])
	}
	cfg.EXPECT().Len().AnyTimes().Return(len(replicas))
	cfg.EXPECT().QuorumSize().AnyTimes().Return(hotstuff.QuorumSize(len(replicas)))
	return cfg, replicas
}

// CreateMockReplica returns a mock of a consensus.Replica.
func CreateMockReplica(t *testing.T, ctrl *gomock.Controller, id hotstuff.ID, key consensus.PublicKey) *mocks.MockReplica {
	t.Helper()

	replica := mocks.NewMockReplica(ctrl)
	replica.
		EXPECT().
		ID().
		AnyTimes().
		Return(id)
	replica.
		EXPECT().
		PublicKey().
		AnyTimes().
		Return(key)

	return replica
}

// ConfigAddReplica adds a mock replica to a mock configuration.
func ConfigAddReplica(t *testing.T, cfg *mocks.MockConfiguration, replica *mocks.MockReplica) {
	t.Helper()

	cfg.
		EXPECT().
		Replica(replica.ID()).
		AnyTimes().
		Return(replica, true)
}

// CreateTCPListener creates a net.Listener on a random port.
func CreateTCPListener(t *testing.T) net.Listener {
	t.Helper()
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	return lis
}

// Sign creates a signature using the given signer.
func Sign(t *testing.T, hash consensus.Hash, signer consensus.Crypto) consensus.Signature {
	t.Helper()
	sig, err := signer.Sign(hash)
	if err != nil {
		t.Fatalf("Failed to sign block: %v", err)
	}
	return sig
}

// CreateSignatures creates partial certificates from multiple signers.
func CreateSignatures(t *testing.T, hash consensus.Hash, signers []consensus.Crypto) []consensus.Signature {
	t.Helper()
	sigs := make([]consensus.Signature, 0, len(signers))
	for _, signer := range signers {
		sigs = append(sigs, Sign(t, hash, signer))
	}
	return sigs
}

// CreateTimeouts creates a set of TimeoutMsg messages from the given signers.
func CreateTimeouts(t *testing.T, view consensus.View, signers []consensus.Crypto) (timeouts []consensus.TimeoutMsg) {
	t.Helper()
	timeouts = make([]consensus.TimeoutMsg, 0, len(signers))
	viewSigs := CreateSignatures(t, view.ToHash(), signers)
	for _, sig := range viewSigs {
		timeouts = append(timeouts, consensus.TimeoutMsg{
			ID:            sig.Signer(),
			View:          view,
			ViewSignature: sig,
			SyncInfo:      consensus.NewSyncInfo().WithQC(consensus.NewQuorumCert(nil, 0, consensus.GetGenesis().Hash())),
		})
	}
	for i := range timeouts {
		timeouts[i].MsgSignature = Sign(t, timeouts[i].Hash(), signers[i])
	}
	return timeouts
}

// CreatePC creates a partial certificate using the given signer.
func CreatePC(t *testing.T, block *consensus.Block, signer consensus.Crypto) consensus.PartialCert {
	t.Helper()
	pc, err := signer.CreatePartialCert(block)
	if err != nil {
		t.Fatalf("Failed to create partial certificate: %v", err)
	}
	return pc
}

// CreatePCs creates one partial certificate using each of the given signers.
func CreatePCs(t *testing.T, block *consensus.Block, signers []consensus.Crypto) []consensus.PartialCert {
	t.Helper()
	pcs := make([]consensus.PartialCert, 0, len(signers))
	for _, signer := range signers {
		pcs = append(pcs, CreatePC(t, block, signer))
	}
	return pcs
}

// CreateQC creates a QC using the given signers.
func CreateQC(t *testing.T, block *consensus.Block, signers []consensus.Crypto) consensus.QuorumCert {
	t.Helper()
	if len(signers) == 0 {
		return consensus.QuorumCert{}
	}
	qc, err := signers[0].CreateQuorumCert(block, CreatePCs(t, block, signers))
	if err != nil {
		t.Fatalf("Failed to create QC: %v", err)
	}
	return qc
}

// CreateTC generates a TC using the given signers.
func CreateTC(t *testing.T, view consensus.View, signers []consensus.Crypto) consensus.TimeoutCert {
	t.Helper()
	if len(signers) == 0 {
		return consensus.TimeoutCert{}
	}
	tc, err := signers[0].CreateTimeoutCert(view, CreateTimeouts(t, view, signers))
	if err != nil {
		t.Fatalf("Failed to create TC: %v", err)
	}
	return tc
}

// GenerateECDSAKey generates an ECDSA private key for use in tests.
func GenerateECDSAKey(t *testing.T) consensus.PrivateKey {
	t.Helper()
	key, err := keygen.GenerateECDSAPrivateKey()
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}
	return key
}

// GenerateBLS12Key generates a BLS12-381 private key for use in tests.
func GenerateBLS12Key(t *testing.T) consensus.PrivateKey {
	t.Helper()
	key, err := bls12.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}
	return key
}

// GenerateKeys generates n keys.
func GenerateKeys(t *testing.T, n int, keyFunc func(t *testing.T) consensus.PrivateKey) (keys []consensus.PrivateKey) {
	keys = make([]consensus.PrivateKey, n)
	for i := 0; i < n; i++ {
		keys[i] = keyFunc(t)
	}
	return keys
}

// NewProposeMsg wraps a new block in a ProposeMsg.
func NewProposeMsg(parent consensus.Hash, qc consensus.QuorumCert, cmd consensus.Command, view consensus.View, id hotstuff.ID) consensus.ProposeMsg {
	return consensus.ProposeMsg{ID: id, Block: consensus.NewBlock(parent, qc, cmd, view, id)}
}

type leaderRotation struct {
	t     *testing.T
	order []hotstuff.ID
}

// GetLeader returns the id of the leader in the given view.
func (l leaderRotation) GetLeader(v consensus.View) hotstuff.ID {
	l.t.Helper()
	if v == 0 {
		l.t.Fatalf("attempt to get leader for view 0")
	}
	if v > consensus.View(len(l.order)) {
		l.t.Fatalf("leader rotation only defined up to view: %v", len(l.order))
	}
	return l.order[v-1]
}

// NewLeaderRotation returns a leader rotation implementation that will return leaders in the specified order.
func NewLeaderRotation(t *testing.T, order ...hotstuff.ID) consensus.LeaderRotation {
	t.Helper()
	return leaderRotation{t, order}
}

// FixedTimeout returns an ExponentialTimeout with a max exponent of 0.
func FixedTimeout(timeout time.Duration) synchronizer.ViewDuration {
	return fixedDuration{timeout}
}

type fixedDuration struct {
	timeout time.Duration
}

func (d fixedDuration) Duration() time.Duration { return d.timeout }
func (d fixedDuration) ViewStarted()            {}
func (d fixedDuration) ViewSucceeded()          {}
func (d fixedDuration) ViewTimeout()            {}
