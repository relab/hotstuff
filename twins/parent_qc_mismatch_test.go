package twins_test

import (
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/protocol"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/leaderrotation"
	"github.com/relab/hotstuff/protocol/rules"
	"github.com/relab/hotstuff/security/crypto"
)

// TestParentQCMismatch verifies that Voter.Verify rejects blocks where
// Block.Parent != Block.QuorumCert().BlockHash().
//
// Attack scenario:
//   - Byzantine leader creates malformed block F with:
//     F.Parent = A.Hash (early block)
//     F.QC = QC(E) (valid QC pointing to latest block E)
//   - This creates inconsistency: Parent chain diverges from QC chain
//   - Without the fix, honest nodes would accept and vote for F
//
// Impact: Blockchain data structure inconsistency, incorrect Extends() results,
// and potential liveness failures.
func TestParentQCMismatch(t *testing.T) {
	for _, cryptoName := range []string{crypto.NameECDSA, crypto.NameEDDSA, crypto.NameBLS12} {
		t.Run(cryptoName, func(t *testing.T) {
			const numReplicas = 4

			dummies := testutil.NewEssentialsSet(t, numReplicas, cryptoName)
			signers := dummies.Signers()
			subject := dummies[0]

			// Build chain: Genesis -> A(V1) -> B(V2) -> C(V3)
			genesisQC := hotstuff.NewQuorumCert(nil, 0, hotstuff.GetGenesis().Hash())

			blockA := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), genesisQC, &clientpb.Batch{}, 1, 1)
			for _, d := range dummies {
				d.Blockchain().Store(blockA)
			}
			qcA := testutil.CreateQC(t, blockA, signers...)

			blockB := hotstuff.NewBlock(blockA.Hash(), qcA, &clientpb.Batch{}, 2, 1)
			for _, d := range dummies {
				d.Blockchain().Store(blockB)
			}
			qcB := testutil.CreateQC(t, blockB, signers...)

			blockC := hotstuff.NewBlock(blockB.Hash(), qcB, &clientpb.Batch{}, 3, 1)
			for _, d := range dummies {
				d.Blockchain().Store(blockC)
			}
			qcC := testutil.CreateQC(t, blockC, signers...)

			// Create malformed block: Parent=Genesis but QC=QC(C)
			// This violates: Block.Parent should equal Block.QC.BlockHash
			malformedBlock := hotstuff.NewBlock(
				hotstuff.GetGenesis().Hash(), // Parent = Genesis (WRONG!)
				qcC,                          // QC points to C
				&clientpb.Batch{},
				4, // View 4
				1, // Byzantine proposer
			)
			for _, d := range dummies {
				d.Blockchain().Store(malformedBlock)
			}

			// Verify the test setup: Parent != QC.BlockHash
			if malformedBlock.Parent() == malformedBlock.QuorumCert().BlockHash() {
				t.Fatal("test setup error: malformed block should have Parent != QC.BlockHash")
			}

			// Setup Voter
			ruler := rules.NewChainedHotStuff(subject.Logger(), subject.RuntimeCfg(), subject.Blockchain())
			viewStates, _ := protocol.NewViewStates(subject.Blockchain(), subject.Authority())
			committer := consensus.NewCommitter(subject.EventLoop(), subject.Logger(), subject.Blockchain(), viewStates, ruler)
			voter := consensus.NewVoter(subject.RuntimeCfg(), leaderrotation.NewFixed(1), ruler, nil, subject.Authority(), committer)

			// Test: Voter.Verify should reject the malformed block
			proposal := hotstuff.ProposeMsg{ID: 1, Block: malformedBlock}
			err := voter.Verify(&proposal)

			if err == nil {
				t.Errorf("Voter.Verify accepted malformed block with Parent != QC.BlockHash\n"+
					"  Parent: %s\n"+
					"  QC.BlockHash: %s\n"+
					"This allows Byzantine leaders to create inconsistent blockchain state.",
					malformedBlock.Parent().SmallString(),
					malformedBlock.QuorumCert().BlockHash().SmallString())
			} else {
				t.Logf("Voter.Verify correctly rejected malformed block: %v", err)
			}
		})
	}
}
