package cert_test

import (
	"os"
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/internal/test"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/security/cert"
	"github.com/relab/hotstuff/security/crypto"
)

func createDummies(t testing.TB, count uint, cryptoName string, cacheSize int) testutil.EssentialsSet {
	opts := make([]core.RuntimeOption, 0)
	if cacheSize > 0 {
		opts = append(opts, core.WithCache(cacheSize))
	}
	return testutil.NewEssentialsSet(t, count, cryptoName, opts...)
}

func createSignersWithBlock(t testing.TB, cryptoName string, cacheSize int) ([]*cert.Authority, *hotstuff.Block) {
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
			modify: func(timeouts []hotstuff.TimeoutMsg, _ []*cert.Authority) []hotstuff.TimeoutMsg {
				return timeouts[:1]
			},
			wantErr: true,
		},
		{
			name: "InvalidQCSignature",
			modify: func(timeouts []hotstuff.TimeoutMsg, _ []*cert.Authority) []hotstuff.TimeoutMsg {
				if len(timeouts) > 0 {
					timeouts[0].MsgSignature = nil
				}
				return timeouts
			},
			wantErr: true,
		},
		{
			name: "NoValidHighQC",
			modify: func(timeouts []hotstuff.TimeoutMsg, _ []*cert.Authority) []hotstuff.TimeoutMsg {
				for i := range timeouts {
					timeouts[i].SyncInfo = hotstuff.NewSyncInfo()
				}
				return timeouts
			},
			wantErr: true,
		},
		{
			name: "EmptyQCs",
			modify: func(_ []hotstuff.TimeoutMsg, _ []*cert.Authority) []hotstuff.TimeoutMsg {
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
			modify: func(agg hotstuff.AggregateQC, _ []*cert.Authority) hotstuff.AggregateQC {
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
			modify: func(agg hotstuff.AggregateQC, _ []*cert.Authority) hotstuff.AggregateQC {
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

func TestVerifyAggregateQC(t *testing.T) {
	type testCase struct {
		cryptoName string
		cacheSize  int
		n          int
		qcsPer     int
	}
	participants := []int{10, 50}
	numQCsPerParticipant := []int{1, 4}

	cases := make([]testCase, 0)
	for _, td := range testData {
		for _, n := range participants {
			for _, qcsPer := range numQCsPerParticipant {
				cases = append(cases, testCase{
					cryptoName: td.cryptoName,
					cacheSize:  td.cacheSize,
					n:          n,
					qcsPer:     qcsPer,
				})
			}
		}
	}
	for _, c := range cases {
		name := test.Name(
			"Crypto", c.cryptoName,
			"Cache", c.cacheSize,
			"Participants", c.n,
			"QCsPerParticipant", c.qcsPer,
		)
		t.Run(name, func(t *testing.T) {
			verifier, aggQC := buildAuthsAndAggregateQC(t, c.n, c.cryptoName, c.cacheSize, c.qcsPer)
			_, err := verifier.VerifyAggregateQC(aggQC)
			if err != nil {
				t.Fatalf("VerifyAggregateQC failed: %v", err)
			}
		})
	}
}

// TestVerifyAggregateQCReproduceBLS12FailVerify attempts to reproduce a failure
// in BLS12 AggregateQC verification that was observed during benchmarking.
// See issue #232. This test is skipped by default and can be run by setting
// the DEBUG environment variable as follows:
//
//	DEBUG=1 go test -v -run TestVerifyAggregateQCReproduceBLS12FailVerify -timeout=0 -count=1 > debug-bls12-verify.log
//
// Adjust the count parameter as needed to increase the likelihood of reproducing the issue.
func TestVerifyAggregateQCReproduceBLS12FailVerify(t *testing.T) {
	if os.Getenv("DEBUG") == "" {
		t.Skip("Skipping slow debug-only test")
	}

	tests := []struct {
		cryptoName string
		cacheSize  int
		n          int
		qcsPer     int
	}{
		{crypto.NameBLS12, 0, 800, 4},
		{crypto.NameBLS12, 10, 800, 1},
		{crypto.NameBLS12, 10, 400, 4},
		{crypto.NameBLS12, 10, 800, 40},
	}
	for _, c := range tests {
		name := test.Name(
			"Crypto", c.cryptoName,
			"Cache", c.cacheSize,
			"Participants", c.n,
			"QCsPerParticipant", c.qcsPer,
		)
		t.Run(name, func(t *testing.T) {
			verifier, aggQC := buildAuthsAndAggregateQC(t, c.n, c.cryptoName, c.cacheSize, c.qcsPer)
			_, err := verifier.VerifyAggregateQC(aggQC)
			if err != nil {
				t.Logf("aggQC: %v", aggQC)
				t.Fatalf("VerifyAggregateQC failed: %v", err)
			}
		})
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

// BenchmarkVerifyAggregateQC benchmarks Authority.VerifyAggregateQC with varying parameters.
func BenchmarkVerifyAggregateQC(b *testing.B) {
	type benchCase struct {
		cryptoName string
		cacheSize  int
		n          int
		qcsPer     int
	}
	participants := []int{10, 100, 200, 400}
	numQCsPerParticipant := []int{1, 4, 20}

	cases := make([]benchCase, 0)
	for _, td := range testData {
		for _, n := range participants {
			for _, qcsPer := range numQCsPerParticipant {
				cases = append(cases, benchCase{
					cryptoName: td.cryptoName,
					cacheSize:  td.cacheSize,
					n:          n,
					qcsPer:     qcsPer,
				})
			}
		}
	}
	for _, c := range cases {
		name := test.Name(
			"Crypto", c.cryptoName,
			"Cache", c.cacheSize,
			"Participants", c.n,
			"QCsPerParticipant", c.qcsPer,
		)
		b.Run(name, func(b *testing.B) {
			verifier, aggQC := buildAuthsAndAggregateQC(b, c.n, c.cryptoName, c.cacheSize, c.qcsPer)
			for b.Loop() {
				_, err := verifier.VerifyAggregateQC(aggQC)
				if err != nil {
					b.Fatalf("VerifyAggregateQC failed: %v", err)
				}
			}
		})
	}
}

// buildAuthsAndAggregateQC creates a verifier authority and an AggregateQC according to the parameters.
// It creates QCs across multiple views to exercise findHighestValidQC logic in the worst-case scenario:
// - Only the lowest view (view 1) has a valid QC
// - All higher views have invalid QCs (blocks not stored in blockchain)
// - This forces findHighestValidQC to iterate through all invalid QCs before finding the valid one
func buildAuthsAndAggregateQC(tb testing.TB, n int, cryptoName string, cacheSize, numQCs int) (*cert.Authority, hotstuff.AggregateQC) {
	tb.Helper()
	if n <= 0 {
		tb.Fatalf("participant count must be > 0")
	}
	dummies := createDummies(tb, uint(n), cryptoName, cacheSize)
	auths := dummies.Signers()

	poolSize := min(numQCs, n)
	qcPool := make([]hotstuff.QuorumCert, 0, poolSize)

	// Views: (poolSize-1)*10+1, ..., 21, 11, 1 (descending)
	for k := poolSize - 1; k >= 0; k-- {
		parentQC := hotstuff.NewQuorumCert(nil, 0, hotstuff.GetGenesis().Hash())
		view := blockView(k)
		block := hotstuff.NewBlock(hotstuff.GetGenesis().Hash(), parentQC, &clientpb.Batch{Commands: []*clientpb.Command{}}, view, hotstuff.ID(1))

		if view == 1 {
			// Valid QC: store the block for view 1
			for _, dummy := range dummies {
				dummy.Blockchain().Store(block)
			}
		}
		// For view > 1, we DON'T store the block, making these QCs invalid during verification

		qc := testutil.CreateQC(tb, block, auths...)
		qcPool = append(qcPool, qc)
	}

	// Distribute QCs among participants to create worst-case for findHighestValidQC.
	// We want the highest views to have the most QCs, forcing maximum iterations.
	// Strategy: Assign high-view (invalid) QCs to most participants, and the valid (view 1) QC to fewer.
	perParticipantQCs := make([]hotstuff.QuorumCert, n)
	for i := range n {
		perParticipantQCs[i] = qcPool[i%poolSize]
	}

	view := blockView(poolSize - 1)
	timeouts := testutil.CreateTimeouts(tb, view, auths, perParticipantQCs...)
	agg, err := auths[0].CreateAggregateQC(view, timeouts)
	if err != nil {
		tb.Fatalf("failed to create AggregateQC: %v", err)
	}
	return auths[0], agg
}

// blockView returns a hotstuff.View for the k-th block in the test setup.
// That is, it returns views: 1, 11, 21, ..., for k = 0, 1, 2, ...
func blockView(k int) hotstuff.View {
	return hotstuff.View(k*10 + 1)
}
