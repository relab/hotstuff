package ecdsa

import (
	"errors"
	"math/big"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/crypto"
	"github.com/relab/hotstuff/internal/mocks"
	"github.com/relab/hotstuff/internal/testutil"
)

func TestCreatePartialCert(t *testing.T) {
	run := func(t *testing.T, newFunc newFunc) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		td := newTestData(t, ctrl, 1, newFunc)

		partialCert, err := td.signers[0].Sign(td.block)
		if err != nil {
			t.Fatalf("Failed to create partial certificate: %v", err)
		}

		if partialCert.BlockHash() != td.block.Hash() {
			t.Error("Partial certificate hash does not match block hash!")
		}

		if signerID := partialCert.Signature().Signer(); signerID != hotstuff.ID(1) {
			t.Errorf("Wrong ID for signer in partial certificate: got: %d, want: %d", signerID, hotstuff.ID(1))
		}
	}
	runBoth(t, run)
}

func TestVerifyPartialCert(t *testing.T) {
	run := func(t *testing.T, newFunc newFunc) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		td := newTestData(t, ctrl, 1, newFunc)
		partialCert := testutil.CreatePC(t, td.block, td.signers[0])

		if !td.verifiers[0].VerifyPartialCert(partialCert) {
			t.Error("Partial Certificate was not verified.")
		}
	}
	runBoth(t, run)
}

func TestVerifyInvalidPartialCert(t *testing.T) {
	run := func(t *testing.T, newFunc newFunc) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		td := newTestData(t, ctrl, 1, newFunc)
		pc := NewPartialCert(NewSignature(big.NewInt(1), big.NewInt(2), 1), td.block.Hash())

		if td.verifiers[0].VerifyPartialCert(pc) {
			t.Error("Invalid partial certificate was verified.")
		}
	}
	runBoth(t, run)
}

func TestCreateQuorumCert(t *testing.T) {
	run := func(t *testing.T, newFunc newFunc) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		td := newTestData(t, ctrl, 4, newFunc)

		pcs := testutil.CreatePCs(t, td.block, td.signers)

		qc, err := td.signers[0].CreateQuorumCert(td.block, pcs)
		if err != nil {
			t.Fatalf("Failed to create QC: %v", err)
		}

		if qc.BlockHash() != td.block.Hash() {
			t.Error("Quorum certificate hash does not match block hash!")
		}

		if len(qc.(*QuorumCert).Signatures()) != len(pcs) {
			t.Error("Quorum certificate does not include all partial certificates!")
		}
	}
	runBoth(t, run)
}

func TestCreateQuorumCertInvalid(t *testing.T) {
	run := func(t *testing.T, newFunc newFunc) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		td := newTestData(t, ctrl, 4, newFunc)

		badHash := append(testutil.CreatePCs(t, td.block, td.signers), testutil.CreatePC(t, hotstuff.GetGenesis(), td.signers[0]))
		duplicate := append(testutil.CreatePCs(t, td.block, td.signers), testutil.CreatePC(t, td.block, td.signers[0]))

		tests := []struct {
			name  string
			votes []hotstuff.PartialCert
			err   error
		}{
			{"hash mismatch", badHash, ErrHashMismatch},
			{"duplicate", duplicate, ErrPartialDuplicate},
		}

		for _, test := range tests {
			_, err := td.signers[0].CreateQuorumCert(td.block, test.votes)
			if err == nil {
				t.Fatalf("%s: Expected CreateQuorumCert to fail, but was successful!", test.name)
			}
			if !errors.Is(err, test.err) {
				t.Fatalf("%s: got: %v, want: %v", test.name, err, test.err)
			}
		}
	}
	runBoth(t, run)
}

func TestVerifyGenesisQC(t *testing.T) {
	run := func(t *testing.T, newFunc newFunc) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		td := newTestData(t, ctrl, 4, newFunc)

		genesisQC := NewQuorumCert(make(map[hotstuff.ID]*Signature), hotstuff.GetGenesis().Hash())

		if !td.verifiers[0].VerifyQuorumCert(genesisQC) {
			t.Error("Genesis QC was not verified!")
		}
	}
	runBoth(t, run)
}

func TestVerifyQuorumCert(t *testing.T) {
	run := func(t *testing.T, newFunc newFunc) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		td := newTestData(t, ctrl, 4, newFunc)

		qc := testutil.CreateQC(t, td.block, td.signers)

		for i, verifier := range td.verifiers {
			if !verifier.VerifyQuorumCert(qc) {
				t.Errorf("Verifier %d failed to verify QC!", i+1)
			}
		}
	}
	runBoth(t, run)
}

func TestVerifyInvalidQuorumCert(t *testing.T) {
	run := func(t *testing.T, newFunc newFunc) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		td := newTestData(t, ctrl, 4, newFunc)
		goodQC := testutil.CreateQC(t, td.block, td.signers)

		tests := []struct {
			name string
			qc   hotstuff.QuorumCert
		}{
			{"empty", NewQuorumCert(make(map[hotstuff.ID]*Signature), td.block.Hash())},
			{"not enough signatures", testutil.CreateQC(t, td.block, td.signers[:2])},
			{"hash mismatch", NewQuorumCert(goodQC.(*QuorumCert).Signatures(), hotstuff.Hash{})},
		}

		for _, test := range tests {
			if td.verifiers[0].VerifyQuorumCert(test.qc) {
				t.Errorf("%s: Expected QC to be invalid, but was validated.", test.name)
			}
		}
	}
	runBoth(t, run)
}

func TestVerifyCachedPartialCert(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	td := newTestData(t, ctrl, 2, newCache)
	// create signature with other signer, so that it is not put in the cache right away.
	pc := testutil.CreatePC(t, td.block, td.signers[1])

	// verify once to put it in the cache
	if !td.verifiers[0].VerifyPartialCert(pc) {
		t.Error("Failed to verify partial cert!")
	}

	// now verify again
	if !td.verifiers[0].VerifyPartialCert(pc) {
		t.Error("Failed to verify partial cert!")
	}
}

func TestVerifyCachedQuorumCert(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	td := newTestData(t, ctrl, 5, newCache)

	// create signature with other signers, so that it is not put in the cache right away.
	qc := testutil.CreateQC(t, td.block, td.signers[1:])

	// verify once to put it in the cache
	if !td.verifiers[0].VerifyQuorumCert(qc) {
		t.Error("Failed to verify partial cert!")
	}

	// now verify again
	if !td.verifiers[0].VerifyQuorumCert(qc) {
		t.Error("Failed to verify partial cert!")
	}
}

func TestCacheDrop(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	td := newTestData(t, ctrl, 5, newCache)

	// create signatures with other signers, so that it is not put in the cache right away.
	qc1 := testutil.CreateQC(t, td.block, td.signers[1:])
	qc2 := testutil.CreateQC(t, td.block, td.signers[1:])

	// verify qc1 to put it in the cache
	if !td.verifiers[0].VerifyQuorumCert(qc1) {
		t.Error("Failed to verify partial cert!")
	}

	// verify qc2 to put it in the cache. Now qc1 signatures should be evicted
	if !td.verifiers[0].VerifyQuorumCert(qc2) {
		t.Error("Failed to verify partial cert!")
	}

	// verify qc1 again
	if !td.verifiers[0].VerifyQuorumCert(qc1) {
		t.Error("Failed to verify partial cert!")
	}
}

func runBoth(t *testing.T, run func(*testing.T, newFunc)) {
	t.Helper()
	t.Run("NoCache", func(t *testing.T) { run(t, New) })
	t.Run("WithCache", func(t *testing.T) { run(t, newCache) })
}

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

func createMockConfig(t *testing.T, ctrl *gomock.Controller, id hotstuff.ID) *mocks.MockConfig {
	t.Helper()
	cfg := testutil.CreateMockConfig(t, ctrl, id, createKey(t))
	cfg.
		EXPECT().
		QuorumSize().
		AnyTimes().
		Return(3)

	return cfg
}

type newFunc func(hotstuff.Config) (hotstuff.Signer, hotstuff.Verifier)

func newCache(cfg hotstuff.Config) (hotstuff.Signer, hotstuff.Verifier) {
	return NewWithCache(cfg, 5)
}

type testData struct {
	signers   []hotstuff.Signer
	verifiers []hotstuff.Verifier
	block     *hotstuff.Block
}

func newTestData(t *testing.T, ctrl *gomock.Controller, n int, newFunc newFunc) testData {
	t.Helper()

	replicas := make([]*mocks.MockReplica, 0, n)
	configs := make([]*mocks.MockConfig, 0, n)
	signers := make([]hotstuff.Signer, 0, n)
	verifiers := make([]hotstuff.Verifier, 0, n)

	for i := 0; i < n; i++ {
		id := hotstuff.ID(i) + 1
		configs = append(configs, createMockConfig(t, ctrl, id))
		replicas = append(replicas, testutil.CreateMockReplica(t, ctrl, id, configs[i].PrivateKey().PublicKey()))
		sign, verify := newFunc(configs[i])
		signers = append(signers, sign)
		verifiers = append(verifiers, verify)
	}

	for _, config := range configs {
		for _, replica := range replicas {
			testutil.ConfigAddReplica(t, config, replica)
		}
	}

	return testData{
		signers:   signers,
		verifiers: verifiers,
		block:     createBlock(t, signers[0]),
	}
}
