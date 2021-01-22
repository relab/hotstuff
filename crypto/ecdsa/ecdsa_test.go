package ecdsa

import (
	"crypto/ecdsa"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/crypto"
	"github.com/relab/hotstuff/internal/mocks"
)

func createKey(t *testing.T) *PrivateKey {
	t.Helper()
	pk, err := crypto.GeneratePrivateKey()
	if err != nil {
		t.Errorf("Failed to generate private key: %v", err)
	}
	return &PrivateKey{pk}
}

func createBlock(t *testing.T, signer hotstuff.Signer) *hotstuff.Block {
	t.Helper()

	qc, err := signer.CreateQuorumCert(hotstuff.GetGenesis(), []hotstuff.PartialCert{})
	if err != nil {
		t.Errorf("Could not create empty QC for genesis: %v", err)
	}

	b := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), qc, "foo", 42, 1)
	return b
}

func createMockReplica(t *testing.T, ctrl *gomock.Controller, id hotstuff.ID, key *ecdsa.PublicKey) *mocks.MockReplica {
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

func createMockConfig(t *testing.T, ctrl *gomock.Controller, id hotstuff.ID, key *PrivateKey) *mocks.MockConfig {
	t.Helper()

	cfg := mocks.NewMockConfig(ctrl)
	cfg.
		EXPECT().
		PrivateKey().
		AnyTimes().
		Return(key)
	cfg.
		EXPECT().
		ID().
		AnyTimes().
		Return(id)
	cfg.
		EXPECT().
		QuorumSize().
		AnyTimes().
		Return(3)

	return cfg
}

func configAddReplica(t *testing.T, cfg *mocks.MockConfig, replica *mocks.MockReplica) {
	t.Helper()

	cfg.
		EXPECT().
		Replica(replica.ID()).
		AnyTimes().
		Return(replica, true)
}

func TestCreateAndVerifyPartialCert(t *testing.T) {

	ctrl := gomock.NewController(t)

	key := createKey(t)
	replica := createMockReplica(t, ctrl, 1, &key.PrivateKey.PublicKey)
	cfg := createMockConfig(t, ctrl, 1, key)
	configAddReplica(t, cfg, replica)

	signer, verifier := New(cfg)

	b := createBlock(t, signer)

	pc, err := signer.Sign(b)
	if err != nil {
		t.Errorf("Could not create partial cert for block: %v", err)
	}

	if !verifier.VerifyPartialCert(pc) {
		t.Error("Partial cert was invalid.")
	}
}

func TestCreateAndVerifyQuorumCert(t *testing.T) {
	ctrl := gomock.NewController(t)

	keys := make([]*PrivateKey, 0, 3)
	replicas := make([]*mocks.MockReplica, 0, 3)
	configs := make([]*mocks.MockConfig, 0, 3)
	signers := make([]hotstuff.Signer, 0, 3)
	verifiers := make([]hotstuff.Verifier, 0, 3)
	pcs := make([]hotstuff.PartialCert, 0, 3)

	for i := 0; i < 3; i++ {
		id := hotstuff.ID(i) + 1
		keys = append(keys, createKey(t))
		replicas = append(replicas, createMockReplica(t, ctrl, id, &keys[i].PrivateKey.PublicKey))
		configs = append(configs, createMockConfig(t, ctrl, id, keys[i]))
		signer, verifier := New(configs[i])
		signers = append(signers, signer)
		verifiers = append(verifiers, verifier)
	}

	for _, config := range configs {
		for _, replica := range replicas {
			configAddReplica(t, config, replica)
		}
	}

	b := createBlock(t, signers[0])
	for _, signer := range signers {
		sig, err := signer.Sign(b)
		if err != nil {
			t.Fatalf("Failed to sign block: %v", err)
		}
		pcs = append(pcs, sig)
	}

	qc, err := signers[0].CreateQuorumCert(b, pcs)
	if err != nil {
		t.Fatalf("Failed to create QC: %v", err)
	}

	for i, verifier := range verifiers {
		if !verifier.VerifyQuorumCert(qc) {
			t.Fatalf("Replica %d failed to verify QC: %v", i+1, err)
		}
	}
}
