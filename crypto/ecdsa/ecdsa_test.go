package ecdsa_test

import (
	"errors"
	"math/big"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/crypto/ecdsa"
	"github.com/relab/hotstuff/internal/testutil"
)

func TestCreatePartialCert(t *testing.T) {
	run := func(t *testing.T, newFunc newFunc) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		td := newTestData(t, ctrl, 1, newFunc)

		partialCert, err := td.signers[0].CreatePartialCert(td.block)
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
		pc := ecdsa.NewPartialCert(ecdsa.NewSignature(big.NewInt(1), big.NewInt(2), 1), td.block.Hash())

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

		if len(qc.(*ecdsa.QuorumCert).Signatures()) != len(pcs) {
			t.Error("Quorum certificate does not include all partial certificates!")
		}
	}
	runBoth(t, run)
}

func TestCreateTimeoutCert(t *testing.T) {
	run := func(t *testing.T, newFunc newFunc) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		td := newTestData(t, ctrl, 4, newFunc)

		timeouts := testutil.CreateTimeouts(t, 1, td.signers)

		tc, err := td.signers[0].CreateTimeoutCert(1, timeouts)
		if err != nil {
			t.Fatalf("Failed to create QC: %v", err)
		}

		if tc.View() != hotstuff.View(1) {
			t.Error("Timeout certificate view does not match original view.")
		}

		if len(tc.(*ecdsa.TimeoutCert).Signatures()) != len(timeouts) {
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
			{"hash mismatch", badHash, ecdsa.ErrHashMismatch},
			{"duplicate", duplicate, ecdsa.ErrPartialDuplicate},
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

func TestCreateTimeoutCertInvalid(t *testing.T) {
	run := func(t *testing.T, newFunc newFunc) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		td := newTestData(t, ctrl, 4, newFunc)

		badView := append(testutil.CreateTimeouts(t, 1, td.signers), testutil.CreateTimeouts(t, 2, td.signers[:1])...)
		duplicate := append(testutil.CreateTimeouts(t, 1, td.signers), testutil.CreateTimeouts(t, 1, td.signers[:1])...)

		tests := []struct {
			name  string
			votes []*hotstuff.TimeoutMsg
			err   error
		}{
			{"hash mismatch", badView, ecdsa.ErrViewMismatch},
			{"duplicate", duplicate, ecdsa.ErrPartialDuplicate},
		}

		for _, test := range tests {
			_, err := td.signers[0].CreateTimeoutCert(1, test.votes)
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

		genesisQC := ecdsa.NewQuorumCert(make(map[hotstuff.ID]*ecdsa.Signature), hotstuff.GetGenesis().Hash())

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
				t.Errorf("verifier %d failed to verify QC!", i+1)
			}
		}
	}
	runBoth(t, run)
}

func TestVerifyTimeoutCert(t *testing.T) {
	run := func(t *testing.T, newFunc newFunc) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		td := newTestData(t, ctrl, 4, newFunc)

		tc := testutil.CreateTC(t, 1, td.signers)

		for i, verifier := range td.verifiers {
			if !verifier.VerifyTimeoutCert(tc) {
				t.Errorf("verifier %d failed to verify TC!", i+1)
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
			{"empty", ecdsa.NewQuorumCert(make(map[hotstuff.ID]*ecdsa.Signature), td.block.Hash())},
			{"not enough signatures", testutil.CreateQC(t, td.block, td.signers[:2])},
			{"hash mismatch", ecdsa.NewQuorumCert(goodQC.(*ecdsa.QuorumCert).Signatures(), hotstuff.Hash{})},
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
	t.Run("NoCache", func(t *testing.T) { run(t, ecdsa.New) })
	t.Run("WithCache", func(t *testing.T) { run(t, newCache) })
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

type newFunc func() (hotstuff.Signer, hotstuff.Verifier)

func newCache() (hotstuff.Signer, hotstuff.Verifier) {
	return ecdsa.NewWithCache(5)
}

type testData struct {
	signers   []hotstuff.Signer
	verifiers []hotstuff.Verifier
	block     *hotstuff.Block
}

func newTestData(t *testing.T, ctrl *gomock.Controller, n int, newFunc newFunc) testData {
	t.Helper()

	bl := testutil.CreateBuilders(t, ctrl, n)
	hl := bl.Build()

	return testData{
		signers:   hl.Signers(),
		verifiers: hl.Verifiers(),
		block:     createBlock(t, hl[0].Signer()),
	}
}
