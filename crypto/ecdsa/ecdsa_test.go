package ecdsa_test

import (
	"math/big"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/crypto/ecdsa"
	"github.com/relab/hotstuff/internal/testutil"
)

func TestVerifyInvalidPartialCert(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	td := newTestData(t, ctrl, 1)
	pc := ecdsa.NewPartialCert(ecdsa.NewSignature(big.NewInt(1), big.NewInt(2), 1), td.block.Hash())

	if td.verifiers[0].VerifyPartialCert(pc) {
		t.Error("Invalid partial certificate was verified.")
	}
}

func TestVerifyInvalidQuorumCert(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	td := newTestData(t, ctrl, 4)
	goodQC := testutil.CreateQC(t, td.block, td.signers[1:])
	signatures := goodQC.(*ecdsa.QuorumCert).Signatures()

	// set up a map of signatures that is too small to be valid
	badSignatures := make(map[hotstuff.ID]*ecdsa.Signature, len(signatures)-1)
	skip := true
	for k, v := range signatures {
		if skip {
			skip = false
			continue
		}
		badSignatures[k] = v
	}

	tests := []struct {
		name string
		qc   hotstuff.QuorumCert
	}{
		{"empty", ecdsa.NewQuorumCert(make(map[hotstuff.ID]*ecdsa.Signature), td.block.Hash())},
		{"not enough signatures", ecdsa.NewQuorumCert(badSignatures, goodQC.BlockHash())},
		{"hash mismatch", ecdsa.NewQuorumCert(signatures, hotstuff.Hash{})},
	}

	for _, test := range tests {
		if td.verifiers[0].VerifyQuorumCert(test.qc) {
			t.Errorf("%s: Expected QC to be invalid, but was validated.", test.name)
		}
	}
}

func TestVerifyCachedPartialCert(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	td := newTestData(t, ctrl, 2)
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

	td := newTestData(t, ctrl, 5)

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

	td := newTestData(t, ctrl, 5)

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

func createBlock(t *testing.T, signer hotstuff.Signer) *hotstuff.Block {
	t.Helper()

	qc, err := signer.CreateQuorumCert(hotstuff.GetGenesis(), []hotstuff.PartialCert{})
	if err != nil {
		t.Errorf("Could not create empty QC for genesis: %v", err)
	}

	b := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), qc, "foo", 42, 1)
	return b
}

type testData struct {
	signers   []hotstuff.Signer
	verifiers []hotstuff.Verifier
	block     *hotstuff.Block
}

func newTestData(t *testing.T, ctrl *gomock.Controller, n int) testData {
	t.Helper()

	bl := testutil.CreateBuilders(t, ctrl, n)
	hl := bl.Build()

	return testData{
		signers:   hl.Signers(),
		verifiers: hl.Verifiers(),
		block:     createBlock(t, hl[0].Signer()),
	}
}
