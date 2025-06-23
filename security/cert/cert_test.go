package cert_test

import (
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/security/cert"
	"github.com/relab/hotstuff/security/crypto/bls12"
	"github.com/relab/hotstuff/security/crypto/ecdsa"
	"github.com/relab/hotstuff/security/crypto/eddsa"
)

func createDummies(t *testing.T, count uint, cryptoName string, cacheSize int) testutil.EssentialsSet {
	opts := make([]cert.Option, 0)
	if cacheSize > 0 {
		opts = append(opts, cert.WithCache(cacheSize))
	}
	return testutil.NewEssentialsSet(t, count, cryptoName, opts...)
}

var testData = []struct {
	cryptoName string
	cacheSize  int
}{
	{cryptoName: ecdsa.ModuleName},
	{cryptoName: eddsa.ModuleName},
	{cryptoName: bls12.ModuleName},
	{cryptoName: ecdsa.ModuleName, cacheSize: 10},
	{cryptoName: eddsa.ModuleName, cacheSize: 10},
	{cryptoName: bls12.ModuleName, cacheSize: 10},
}

func TestCreatePartialCert(t *testing.T) {
	for _, td := range testData {
		id := 1
		dummies := createDummies(t, 4, td.cryptoName, td.cacheSize)
		subject := dummies[0]
		block, ok := subject.BlockChain().Get(hotstuff.GetGenesis().Hash())
		if !ok {
			t.Errorf("no block")
		}

		partialCert, err := subject.Authority().CreatePartialCert(block)
		if err != nil {
			t.Fatalf("Failed to create partial certificate: %v", err)
		}

		if partialCert.BlockHash() != block.Hash() {
			t.Error("Partial certificate hash does not match block hash!")
		}

		if signerID := partialCert.Signer(); signerID != hotstuff.ID(id) {
			t.Errorf("Wrong ID for signer in partial certificate: got: %d, want: %d", signerID, hotstuff.ID(id))
		}
	}
}

func TestVerifyPartialCert(t *testing.T) {
	for _, td := range testData {
		dummies := createDummies(t, 2, td.cryptoName, td.cacheSize)
		dummy := dummies[0]
		block := testutil.CreateBlock(t, dummy.Authority())
		dummy.BlockChain().Store(block)

		partialCert := testutil.CreatePC(t, block, dummy.Authority())

		if err := dummy.Authority().VerifyPartialCert(partialCert); err != nil {
			t.Error(err)
		}
	}
}

func TestCreateQuorumCert(t *testing.T) {
	for _, td := range testData {
		const n = 4
		dummies := createDummies(t, n, td.cryptoName, td.cacheSize)
		signers := dummies.Signers()
		dummy := dummies[0]
		block := testutil.CreateBlock(t, dummy.Authority())
		qc := testutil.CreateQC(t, block, signers...)
		if qc.BlockHash() != block.Hash() {
			t.Error("Quorum certificate hash does not match block hash!")
		}
	}
}

func TestCreateTimeoutCert(t *testing.T) {
	for _, td := range testData {
		const n = 4
		dummies := createDummies(t, n, td.cryptoName, td.cacheSize)
		signers := dummies.Signers()

		tc := testutil.CreateTC(t, 1, signers)
		if tc.View() != hotstuff.View(1) {
			t.Error("Timeout certificate view does not match original view.")
		}
	}
}

func TestCreateQCWithOneSig(t *testing.T) {
	for _, td := range testData {
		const n = 4
		dummies := createDummies(t, n, td.cryptoName, td.cacheSize)
		signers := dummies.Signers()
		dummy := dummies[0]
		block := testutil.CreateBlock(t, dummy.Authority())
		pcs := testutil.CreatePCs(t, block, signers)
		_, err := signers[0].CreateQuorumCert(block, pcs[:1])
		if err == nil {
			t.Fatal("Expected error when creating QC with only one signature")
		}
	}
}

func TestCreateQCWithOverlappingSigs(t *testing.T) {
	for _, td := range testData {
		const n = 4
		dummies := createDummies(t, n, td.cryptoName, td.cacheSize)
		signers := dummies.Signers()
		dummy := dummies[0]
		block := testutil.CreateBlock(t, dummy.Authority())
		pcs := testutil.CreatePCs(t, block, signers)
		pcs = append(pcs, pcs[0])
		_, err := signers[0].CreateQuorumCert(block, pcs)
		if err == nil {
			t.Fatal("Expected error when creating QC with overlapping signatures")
		}
	}
}

func TestVerifyGenesisQC(t *testing.T) {
	for _, td := range testData {
		const n = 4
		dummies := createDummies(t, n, td.cryptoName, td.cacheSize)
		signers := dummies.Signers()
		genesisQC := testutil.CreateQC(t, hotstuff.GetGenesis(), signers[0])
		if err := signers[1].VerifyQuorumCert(genesisQC); err != nil {
			t.Error(err)
		}
	}
}

func TestVerifyQuorumCert(t *testing.T) {
	for _, td := range testData {
		const n = 4
		dummies := createDummies(t, n, td.cryptoName, td.cacheSize)
		signers := dummies.Signers()
		signedBlock := testutil.CreateBlock(t, dummies[0].Authority())
		for _, dummy := range dummies {
			dummy.BlockChain().Store(signedBlock)
		}

		qc := testutil.CreateQC(t, signedBlock, signers...)

		for i, verifier := range signers {
			if err := verifier.VerifyQuorumCert(qc); err != nil {
				t.Errorf("verifier %d failed to verify QC: %v", i+1, err)
			}
		}
	}
}

func TestVerifyTimeoutCert(t *testing.T) {
	for _, td := range testData {
		const n = 4
		dummies := createDummies(t, n, td.cryptoName, td.cacheSize)
		signers := dummies.Signers()
		signedBlock := testutil.CreateBlock(t, dummies[0].Authority())
		for _, dummy := range dummies {
			dummy.BlockChain().Store(signedBlock)
		}

		tc := testutil.CreateTC(t, 1, signers)

		for i, verifier := range signers {
			if err := verifier.VerifyTimeoutCert(tc); err != nil {
				t.Errorf("verifier %d failed to verify TC: %v", i+1, err)
			}
		}
	}
}

func TestVerifyAggregateQC(t *testing.T) {
	for _, td := range testData {
		const n = 4
		dummies := createDummies(t, n, td.cryptoName, td.cacheSize)
		signers := dummies.Signers()
		signedBlock := testutil.CreateBlock(t, dummies[0].Authority())
		for _, dummy := range dummies {
			dummy.BlockChain().Store(signedBlock)
		}

		timeouts := testutil.CreateTimeouts(t, 1, signers)
		aggQC, err := signers[0].CreateAggregateQC(1, timeouts)
		if err != nil {
			t.Fatal(err)
		}

		highQC, err := signers[0].VerifyAggregateQC(aggQC)
		if err != nil {
			t.Fatalf("AggregateQC was not verified: %v", err)
		}

		if highQC.BlockHash() != hotstuff.GetGenesis().Hash() {
			t.Fatal("Wrong hash for highQC")
		}
	}
}
