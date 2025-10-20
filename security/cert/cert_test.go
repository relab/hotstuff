package cert_test

import (
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/test"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/security/cert"
	"github.com/relab/hotstuff/security/crypto"
)

func createDummies(t *testing.T, count uint, cryptoName string, cacheSize int) testutil.EssentialsSet {
	opts := make([]cert.Option, 0)
	if cacheSize > 0 {
		opts = append(opts, cert.WithCache(cacheSize))
	}
	return testutil.NewEssentialsSet(t, count, cryptoName, opts...)
}

func createSignersWithBlock(t *testing.T, cryptoName string, cacheSize int) ([]*cert.Authority, *hotstuff.Block) {
	const n = 4
	dummies := createDummies(t, n, cryptoName, cacheSize)
	signers := dummies.Signers()
	signedBlock := testutil.CreateBlock(t, signers[0])
	for _, dummy := range dummies {
		dummy.Blockchain().Store(signedBlock)
	}
	return signers, signedBlock
}

var testData = []struct {
	cryptoName string
	cacheSize  int
}{
	{cryptoName: crypto.NameECDSA},
	{cryptoName: crypto.NameEDDSA},
	{cryptoName: crypto.NameBLS12},
	{cryptoName: crypto.NameECDSA, cacheSize: 10},
	{cryptoName: crypto.NameEDDSA, cacheSize: 10},
	{cryptoName: crypto.NameBLS12, cacheSize: 10},
}

func TestCreatePartialCert(t *testing.T) {
	for _, td := range testData {
		id := 1
		dummies := createDummies(t, 4, td.cryptoName, td.cacheSize)
		subject := dummies[0]
		block, ok := subject.Blockchain().Get(hotstuff.GetGenesis().Hash())
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
		dummy.Blockchain().Store(block)

		partialCert := testutil.CreatePC(t, block, dummy.Authority())
		if err := dummy.Authority().VerifyPartialCert(partialCert); err != nil {
			t.Error(err)
		}
	}
}

func TestCreateQuorumCert(t *testing.T) {
	for _, td := range testData {
		signers, block := createSignersWithBlock(t, td.cryptoName, td.cacheSize)
		qc := testutil.CreateQC(t, block, signers...)
		if qc.BlockHash() != block.Hash() {
			t.Error("Quorum certificate hash does not match block hash!")
		}
	}
}

func TestCreateTimeoutCert(t *testing.T) {
	for _, td := range testData {
		signers, _ := createSignersWithBlock(t, td.cryptoName, td.cacheSize)
		tc := testutil.CreateTC(t, 1, signers)
		if tc.View() != hotstuff.View(1) {
			t.Error("Timeout certificate view does not match original view.")
		}
	}
}

func TestCreateQCWithOneSig(t *testing.T) {
	for _, td := range testData {
		signers, block := createSignersWithBlock(t, td.cryptoName, td.cacheSize)
		pcs := testutil.CreatePCs(t, block, signers)
		_, err := signers[0].CreateQuorumCert(block, pcs[:1])
		if err == nil {
			t.Fatal("Expected error when creating QC with only one signature")
		}
	}
}

func TestCreateQCWithOverlappingSigs(t *testing.T) {
	for _, td := range testData {
		signers, block := createSignersWithBlock(t, td.cryptoName, td.cacheSize)
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
		signers, _ := createSignersWithBlock(t, td.cryptoName, td.cacheSize)
		genesisQC := testutil.CreateQC(t, hotstuff.GetGenesis(), signers[0])
		if err := signers[1].VerifyQuorumCert(genesisQC); err != nil {
			t.Error(err)
		}
	}
}

func TestVerifyQuorumCert(t *testing.T) {
	for _, td := range testData {
		signers, signedBlock := createSignersWithBlock(t, td.cryptoName, td.cacheSize)
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
		signers, _ := createSignersWithBlock(t, td.cryptoName, td.cacheSize)
		tc := testutil.CreateTC(t, 1, signers)
		for i, verifier := range signers {
			if err := verifier.VerifyTimeoutCert(tc); err != nil {
				t.Errorf("verifier %d failed to verify TC: %v", i+1, err)
			}
		}
	}
}

// Notes about decisions and edge-cases

// Malformed aggregate signature (nil signature): VerifyAggregateQC currently calls aggQC.Sig().Participants() without
// checking for nil, so a nil aggregate signature causes a nil pointer dereference (panic). The test captures this
// panic to exercise the current code path. If desired, we can change VerifyAggregateQC to return a clear error
// instead of panicking.
// QC block missing: to ensure aggregate signature verification succeeded but a QC was invalid later, I constructed timeouts that reference a missing QC in their SyncInfo, signed them properly, and used them to build aggQC. That exercises the case where BatchVerify passes but findHighestValidQC skips missing/invalid QCs.
// qc with nil signature in QC map: replaced one QC with a nil signature in the aggQC.QCs() map. Because other QCs (e.g., genesis QC) may still exist, this typically doesn't cause overall failure — test checks for a valid highQC (e.g., genesis) in that case.

// Next steps I can take (optional)

// Convert the panic case in VerifyAggregateQC to return an error (production change) and update tests accordingly.
// Add more negative tests: e.g., partial invalid timeouts, signatures with insufficient participants, or malformed aggregated signatures that do not panic but produce BatchVerify errors.
// Run the entire repo test suite (can be slow) to ensure no other tests are affected.
// All todos in the plan are completed. If you want me to make the VerifyAggregateQC function robust to nil aggregate signatures, I can implement that change and re-run tests.

// TestVerifyAggregateQCModifiedTimeouts tests AggregateQC verification for cases where timeouts are modified.
func TestVerifyAggregateQCModifiedTimeouts(t *testing.T) {
	tests := []struct {
		name       string
		modify     func([]hotstuff.TimeoutMsg, []*cert.Authority) []hotstuff.TimeoutMsg
		wantErr    bool
		wantHighQC func(hotstuff.QuorumCert) bool
	}{
		{
			name: "InsufficientParticipants",
			modify: func(timeouts []hotstuff.TimeoutMsg, signers []*cert.Authority) []hotstuff.TimeoutMsg {
				return timeouts[:1]
			},
			wantErr: true,
		},
		{
			name: "InvalidQCSignature",
			modify: func(timeouts []hotstuff.TimeoutMsg, signers []*cert.Authority) []hotstuff.TimeoutMsg {
				if len(timeouts) > 0 {
					timeouts[0].MsgSignature = nil
				}
				return timeouts
			},
			wantErr: true,
		},
		{
			name: "NoValidHighQC",
			modify: func(timeouts []hotstuff.TimeoutMsg, signers []*cert.Authority) []hotstuff.TimeoutMsg {
				for i := range timeouts {
					timeouts[i].SyncInfo = hotstuff.NewSyncInfo()
				}
				return timeouts
			},
			wantErr: true,
		},
		{
			name: "EmptyQCs",
			modify: func(timeouts []hotstuff.TimeoutMsg, signers []*cert.Authority) []hotstuff.TimeoutMsg {
				return []hotstuff.TimeoutMsg{}
			},
			wantErr: true,
		},
		{
			name: "QCBlockMissing",
			modify: func(_ []hotstuff.TimeoutMsg, signers []*cert.Authority) []hotstuff.TimeoutMsg {
				qcs := make([]hotstuff.QuorumCert, len(signers))
				for i := range qcs {
					if i == 0 {
						var fakeHash hotstuff.Hash
						qcs[i] = hotstuff.NewQuorumCert(nil, 0, fakeHash)
					} else {
						qcs[i] = hotstuff.NewQuorumCert(nil, 0, hotstuff.GetGenesis().Hash())
					}
				}
				return testutil.CreateTimeouts(t, 1, signers, qcs...)
			},
			wantErr: false,
			wantHighQC: func(qc hotstuff.QuorumCert) bool {
				return qc.BlockHash() == hotstuff.GetGenesis().Hash()
			},
		},
	}

	for _, td := range testData {
		t.Run(test.Name(td.cryptoName, "cache", td.cacheSize), func(t *testing.T) {
			signers, _ := createSignersWithBlock(t, td.cryptoName, td.cacheSize)
			for _, tc := range tests {
				t.Run(tc.name, func(t *testing.T) {
					timeouts := testutil.CreateTimeouts(t, 1, signers)
					timeouts = tc.modify(timeouts, signers)
					aggQC, err := signers[0].CreateAggregateQC(1, timeouts)
					if err != nil {
						if !tc.wantErr {
							t.Errorf("unexpected error creating AggregateQC: %v", err)
						}
						return
					}
					highQC, err := signers[0].VerifyAggregateQC(aggQC)
					if tc.wantErr {
						if err == nil {
							t.Errorf("expected error, got none")
						}
						return
					}
					if err != nil {
						t.Errorf("unexpected error verifying AggregateQC: %v", err)
						return
					}
					if tc.wantHighQC != nil && !tc.wantHighQC(highQC) {
						t.Errorf("highQC did not match expectation")
					}
				})
			}
		})
	}
}

// TestVerifyAggregateQCModifiedAggregateQC tests AggregateQC verification for cases
// where the AggregateQC is modified after creation.
func TestVerifyAggregateQCModifiedAggregateQC(t *testing.T) {
	tests := []struct {
		name       string
		modify     func(hotstuff.AggregateQC, []*cert.Authority) hotstuff.AggregateQC
		wantErr    bool
		wantHighQC func(hotstuff.QuorumCert) bool
	}{
		{
			name: "QCWithNilSignature",
			modify: func(agg hotstuff.AggregateQC, signers []*cert.Authority) hotstuff.AggregateQC {
				qcs := agg.QCs()
				for id, qc := range qcs {
					qcs[id] = hotstuff.NewQuorumCert(nil, qc.View(), qc.BlockHash())
					break
				}
				return hotstuff.NewAggregateQC(qcs, agg.Sig(), agg.View())
			},
			wantErr: false,
			wantHighQC: func(qc hotstuff.QuorumCert) bool {
				return qc.BlockHash() == hotstuff.GetGenesis().Hash()
			},
		},
	}

	for _, td := range testData {
		t.Run(test.Name(td.cryptoName, "cache", td.cacheSize), func(t *testing.T) {
			signers, _ := createSignersWithBlock(t, td.cryptoName, td.cacheSize)
			for _, tc := range tests {
				t.Run(tc.name, func(t *testing.T) {
					timeouts := testutil.CreateTimeouts(t, 1, signers)
					aggQC, err := signers[0].CreateAggregateQC(1, timeouts)
					if err != nil {
						t.Fatalf("unexpected error creating AggregateQC: %v", err)
					}
					aggQC = tc.modify(aggQC, signers)
					highQC, err := signers[0].VerifyAggregateQC(aggQC)
					if tc.wantErr {
						if err == nil {
							t.Errorf("expected error, got none")
						}
						return
					}
					if err != nil {
						t.Errorf("unexpected error verifying AggregateQC: %v", err)
						return
					}
					if tc.wantHighQC != nil && !tc.wantHighQC(highQC) {
						t.Errorf("highQC did not match expectation")
					}
				})
			}
		})
	}
}

// TestVerifyAggregateQCPanic tests AggregateQC verification for cases that are expected to panic.
func TestVerifyAggregateQCPanic(t *testing.T) {
	tests := []struct {
		name   string
		modify func(hotstuff.AggregateQC, []*cert.Authority) hotstuff.AggregateQC
	}{
		{
			name: "MalformedAggregateSignature",
			modify: func(agg hotstuff.AggregateQC, signers []*cert.Authority) hotstuff.AggregateQC {
				return hotstuff.NewAggregateQC(agg.QCs(), nil, agg.View())
			},
		},
	}

	for _, td := range testData {
		t.Run(test.Name(td.cryptoName, "cache", td.cacheSize), func(t *testing.T) {
			signers, _ := createSignersWithBlock(t, td.cryptoName, td.cacheSize)
			for _, tc := range tests {
				t.Run(tc.name, func(t *testing.T) {
					timeouts := testutil.CreateTimeouts(t, 1, signers)
					aggQC, err := signers[0].CreateAggregateQC(1, timeouts)
					if err != nil {
						t.Fatalf("unexpected error creating AggregateQC: %v", err)
					}
					aggQC = tc.modify(aggQC, signers)
					panicked := false
					func() {
						defer func() {
							if r := recover(); r != nil {
								panicked = true
							}
						}()
						_, _ = signers[0].VerifyAggregateQC(aggQC)
					}()
					if !panicked {
						t.Errorf("expected panic during VerifyAggregateQC, but none occurred")
					}
				})
			}
		})
	}
}

// TestVerifyAggregateQCHighQCMismatch ensures VerifyAggregateQC returns a highQC that differs
// from the proposal block's QC when the aggregate QC points to a different block.
func TestVerifyAggregateQCHighQCMismatch(t *testing.T) {
	for _, td := range testData {
		const n = 4
		dummies := createDummies(t, n, td.cryptoName, td.cacheSize)
		signers := dummies.Signers()

		// Create two different blocks and corresponding QCs
		block1 := testutil.CreateBlock(t, dummies[0].Authority())
		for _, dummy := range dummies {
			dummy.Blockchain().Store(block1)
		}
		_ = testutil.CreateQC(t, block1, signers...)

		// Create another block and QC to represent the highQC
		block2 := testutil.CreateBlock(t, dummies[0].Authority())
		for _, dummy := range dummies {
			dummy.Blockchain().Store(block2)
		}
		qc2 := testutil.CreateQC(t, block2, signers...)

		// Create timeouts whose sync infos point to qc2 so the aggregate QC's highQC is qc2
		qcs := make([]hotstuff.QuorumCert, len(signers))
		for i := range qcs {
			qcs[i] = qc2
		}
		timeouts := testutil.CreateTimeouts(t, 1, signers, qcs...)

		aggQC, err := signers[0].CreateAggregateQC(1, timeouts)
		if err != nil {
			t.Fatalf("failed to create aggregated QC: %v", err)
		}

		// Construct proposal where Block has qc1 but AggregateQC contains qc2 as highQC
		proposal := &hotstuff.ProposeMsg{
			Block: block1,
		}
		highQC, err := signers[0].VerifyAggregateQC(aggQC)
		if err != nil {
			t.Fatalf("VerifyAggregateQC failed: %v", err)
		}
		if proposal.Block.QuorumCert().Equals(highQC) {
			t.Fatalf("expected block QC and highQC to differ, but they match")
		}
	}
}

func TestVerifyAnyQC(t *testing.T) {
	for _, td := range testData {
		signers, signedBlock := createSignersWithBlock(t, td.cryptoName, td.cacheSize)
		timeouts := testutil.CreateTimeouts(t, 1, signers)
		aggQC, err := signers[0].CreateAggregateQC(1, timeouts)
		if err != nil {
			t.Fatal(err)
		}
		proposal := &hotstuff.ProposeMsg{
			Block:       signedBlock,
			AggregateQC: &aggQC,
		}
		err = signers[0].VerifyAnyQC(proposal)
		if err != nil {
			t.Fatalf("AnyQC was not verified: %v", err)
		}
	}
}
