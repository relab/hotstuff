package crypto_test

import (
	"testing"

	"github.com/relab/hotstuff/modules"

	"github.com/golang/mock/gomock"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/crypto"
	"github.com/relab/hotstuff/crypto/bls12"
	"github.com/relab/hotstuff/crypto/ecdsa"
	"github.com/relab/hotstuff/internal/testutil"
)

func TestCreatePartialCert(t *testing.T) {
	run := func(t *testing.T, setup setupFunc) {
		ctrl := gomock.NewController(t)

		td := setup(t, ctrl, 1)

		partialCert, err := td.signers[0].CreatePartialCert(td.block)
		if err != nil {
			t.Fatalf("Failed to create partial certificate: %v", err)
		}

		if partialCert.BlockHash() != td.block.Hash() {
			t.Error("Partial certificate hash does not match block hash!")
		}

		if signerID := partialCert.Signer(); signerID != hotstuff.ID(1) {
			t.Errorf("Wrong ID for signer in partial certificate: got: %d, want: %d", signerID, hotstuff.ID(1))
		}
	}
	runAll(t, run)
}

func TestVerifyPartialCert(t *testing.T) {
	run := func(t *testing.T, setup setupFunc) {
		ctrl := gomock.NewController(t)

		td := setup(t, ctrl, 1)
		partialCert := testutil.CreatePC(t, td.block, td.signers[0])

		if !td.verifiers[0].VerifyPartialCert(partialCert) {
			t.Error("Partial Certificate was not verified.")
		}
	}
	runAll(t, run)
}

func TestCreateQuorumCert(t *testing.T) {
	run := func(t *testing.T, setup setupFunc) {
		ctrl := gomock.NewController(t)

		td := setup(t, ctrl, 4)

		pcs := testutil.CreatePCs(t, td.block, td.signers)

		qc, err := td.signers[0].CreateQuorumCert(td.block, pcs)
		if err != nil {
			t.Fatalf("Failed to create QC: %v", err)
		}

		if qc.BlockHash() != td.block.Hash() {
			t.Error("Quorum certificate hash does not match block hash!")
		}
	}
	runAll(t, run)
}

func TestCreateTimeoutCert(t *testing.T) {
	run := func(t *testing.T, setup setupFunc) {
		ctrl := gomock.NewController(t)

		td := setup(t, ctrl, 4)

		timeouts := testutil.CreateTimeouts(t, 1, td.signers)

		tc, err := td.signers[0].CreateTimeoutCert(1, timeouts)
		if err != nil {
			t.Fatalf("Failed to create QC: %v", err)
		}

		if tc.View() != hotstuff.View(1) {
			t.Error("Timeout certificate view does not match original view.")
		}
	}
	runAll(t, run)
}

func TestVerifyGenesisQC(t *testing.T) {
	run := func(t *testing.T, setup setupFunc) {
		ctrl := gomock.NewController(t)

		td := setup(t, ctrl, 4)

		genesisQC, err := td.signers[0].CreateQuorumCert(hotstuff.GetGenesis(), []hotstuff.PartialCert{})
		if err != nil {
			t.Fatal(err)
		}
		if !td.verifiers[0].VerifyQuorumCert(genesisQC) {
			t.Error("Genesis QC was not verified!")
		}
	}
	runAll(t, run)
}

func TestVerifyQuorumCert(t *testing.T) {
	run := func(t *testing.T, setup setupFunc) {
		ctrl := gomock.NewController(t)

		td := setup(t, ctrl, 4)

		qc := testutil.CreateQC(t, td.block, td.signers)

		for i, verifier := range td.verifiers {
			if !verifier.VerifyQuorumCert(qc) {
				t.Errorf("verifier %d failed to verify QC!", i+1)
			}
		}
	}
	runAll(t, run)
}

func TestVerifyTimeoutCert(t *testing.T) {
	run := func(t *testing.T, setup setupFunc) {
		ctrl := gomock.NewController(t)

		td := setup(t, ctrl, 4)

		tc := testutil.CreateTC(t, 1, td.signers)

		for i, verifier := range td.verifiers {
			if !verifier.VerifyTimeoutCert(tc) {
				t.Errorf("verifier %d failed to verify TC!", i+1)
			}
		}
	}
	runAll(t, run)
}

func TestVerifyAggregateQC(t *testing.T) {
	run := func(t *testing.T, setup setupFunc) {
		ctrl := gomock.NewController(t)
		td := setup(t, ctrl, 4)

		timeouts := testutil.CreateTimeouts(t, 1, td.signers)
		aggQC, err := td.signers[0].CreateAggregateQC(1, timeouts)
		if err != nil {
			t.Fatal(err)
		}

		highQC, ok := td.signers[0].VerifyAggregateQC(aggQC)
		if !ok {
			t.Fatal("AggregateQC was not verified")
		}

		if highQC.BlockHash() != hotstuff.GetGenesis().Hash() {
			t.Fatal("Wrong hash for highQC")
		}
	}
	runAll(t, run)
}

func runAll(t *testing.T, run func(*testing.T, setupFunc)) {
	t.Helper()
	t.Run("Ecdsa", func(t *testing.T) { run(t, setup(NewBase(ecdsa.New), testutil.GenerateECDSAKey)) })
	t.Run("Cache+Ecdsa", func(t *testing.T) { run(t, setup(NewCache(ecdsa.New), testutil.GenerateECDSAKey)) })
	t.Run("BLS12-381", func(t *testing.T) { run(t, setup(NewBase(bls12.New), testutil.GenerateBLS12Key)) })
	t.Run("Cache+BLS12-381", func(t *testing.T) { run(t, setup(NewCache(bls12.New), testutil.GenerateBLS12Key)) })
}

func createBlock(t *testing.T, signer modules.Crypto) *hotstuff.Block {
	t.Helper()

	qc, err := signer.CreateQuorumCert(hotstuff.GetGenesis(), []hotstuff.PartialCert{})
	if err != nil {
		t.Errorf("Could not create empty QC for genesis: %v", err)
	}

	b := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), qc, "foo", 42, 1)
	return b
}

type keyFunc func(t *testing.T) hotstuff.PrivateKey
type setupFunc func(*testing.T, *gomock.Controller, int) testData

func setup(newFunc func() modules.Crypto, keyFunc keyFunc) setupFunc {
	return func(t *testing.T, ctrl *gomock.Controller, n int) testData {
		return newTestData(t, ctrl, n, newFunc, keyFunc)
	}
}

func NewCache(impl func() modules.CryptoBase) func() modules.Crypto {
	return func() modules.Crypto {
		return crypto.NewCache(impl(), 10)
	}
}

func NewBase(impl func() modules.CryptoBase) func() modules.Crypto {
	return func() modules.Crypto {
		return crypto.New(impl())
	}
}

type testData struct {
	signers   []modules.Crypto
	verifiers []modules.Crypto
	block     *hotstuff.Block
}

func newTestData(t *testing.T, ctrl *gomock.Controller, n int, newFunc func() modules.Crypto, keyFunc keyFunc) testData {
	t.Helper()

	bl := testutil.CreateBuilders(t, ctrl, n, testutil.GenerateKeys(t, n, keyFunc)...)
	for _, builder := range bl {
		signer := newFunc()
		builder.Add(signer)
	}
	hl := bl.Build()

	var signer modules.Crypto
	hl[0].Get(&signer)

	block := createBlock(t, signer)

	for _, mods := range hl {
		var blockChain modules.BlockChain
		mods.Get(&blockChain)

		blockChain.Store(block)
	}

	return testData{
		signers:   hl.Signers(),
		verifiers: hl.Verifiers(),
		block:     block,
	}
}
