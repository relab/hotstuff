package cert_test

import (
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/security/crypto"
)

// TestVerifyQuorumCert_SignatureReplayAttack tests that QC verification rejects
// forged QCs where the View field has been tampered while reusing valid signatures.
//
// Attack scenario:
//  1. Attacker obtains a legitimate QC for Block A at View 10
//  2. Attacker creates a fake QC with: same signatures, same BlockHash, but View=100
//  3. If verification only checks block content (not view), the fake QC passes
//
// This test ensures the fix in VerifyQuorumCert correctly validates QC.View == Block.View.
func TestVerifyQuorumCert_SignatureReplayAttack(t *testing.T) {
	for _, cryptoName := range []string{crypto.NameECDSA, crypto.NameEDDSA, crypto.NameBLS12} {
		t.Run(cryptoName, func(t *testing.T) {
			const numReplicas = 4
			const originalView = hotstuff.View(10)
			const fakeView = hotstuff.View(100)

			// Setup: Create a legitimate QC at View 10
			dummies := testutil.NewEssentialsSet(t, numReplicas, cryptoName)
			signers := dummies.Signers()

			genesisQC := hotstuff.NewQuorumCert(nil, 0, hotstuff.GetGenesis().Hash())
			block := hotstuff.NewBlock(
				hotstuff.GetGenesis().Hash(),
				genesisQC,
				nil,
				originalView,
				hotstuff.ID(1),
			)

			for _, dummy := range dummies {
				dummy.Blockchain().Store(block)
			}

			// Create legitimate QC
			legitimateQC := testutil.CreateQC(t, block, signers...)

			// Verify original QC is valid
			if err := signers[0].VerifyQuorumCert(legitimateQC); err != nil {
				t.Fatalf("Legitimate QC should be valid: %v", err)
			}

			// Attack: Create fake QC with tampered View but same signatures
			fakeQC := hotstuff.NewQuorumCert(
				legitimateQC.Signature(),
				fakeView, // Tampered: View 10 -> View 100
				legitimateQC.BlockHash(),
			)

			// Verification should fail for the tampered QC
			err := signers[0].VerifyQuorumCert(fakeQC)
			if err == nil {
				t.Errorf("VULNERABILITY: QC with tampered View (%d->%d) was accepted; "+
					"VerifyQuorumCert should reject QCs where QC.View != Block.View",
					originalView, fakeView)
			}
		})
	}
}
