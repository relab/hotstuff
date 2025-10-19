package crypto_test

import (
	"bytes"
	"slices"
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/internal/test"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/security/cert"
	"github.com/relab/hotstuff/security/crypto"
)

// createTestSigners creates n signers with sequential IDs starting from 1.
func createTestSigners(t testing.TB, n int) []*cert.Authority {
	t.Helper()
	signers := make([]*cert.Authority, n)
	for i := range n {
		key := testutil.GenerateECDSAKey(t)
		cfg := core.NewRuntimeConfig(hotstuff.ID(i+1), key)
		base, err := crypto.New(cfg, crypto.NameECDSA)
		if err != nil {
			t.Fatal(err)
		}
		signers[i] = cert.NewAuthority(cfg, nil, base)
	}
	return signers
}

// createQCFromSigners creates a QC signed by all the given signers.
func createQCFromSigners(t testing.TB, signers []*cert.Authority) hotstuff.QuorumCert {
	t.Helper()
	block := testutil.CreateBlock(t, signers[0])
	return testutil.CreateQC(t, block, signers...)
}

// createMultiSignature returns a Multi signature for numSigners sequential IDs,
// or nil (an empty signature) if numSigners == 0.
func createMultiSignature(t testing.TB, numSigners int) crypto.Multi[*crypto.ECDSASignature] {
	t.Helper()
	if numSigners == 0 {
		return nil
	}
	signers := createTestSigners(t, numSigners)
	qc := createQCFromSigners(t, signers)
	sig := qc.Signature()
	multi, ok := sig.(crypto.Multi[*crypto.ECDSASignature])
	if !ok {
		t.Fatalf("Expected signature to be of type Multi[*ECDSASignature], got %T", sig)
	}
	return multi
}
func TestNewMultiSorted(t *testing.T) {
	ms1 := createMultiSignature(t, 2)
	ms2 := slices.Clone(ms1)
	slices.Reverse(ms2)

	cmp := func(a, b *crypto.ECDSASignature) int { return int(a.Signer()) - int(b.Signer()) }

	// Verify that ms1 is sorted and ms2 is not
	if !slices.IsSortedFunc(ms1, cmp) {
		t.Error("ms1 is not sorted: should be sorted")
	}
	if slices.IsSortedFunc(ms2, cmp) {
		t.Error("ms2 is sorted: should be not be sorted")
	}

	// NewMulti should produce sorted output when given sorted input
	sorted := crypto.NewMulti(ms1...)
	if !slices.IsSortedFunc(sorted, cmp) {
		t.Error("NewMulti output is not sorted for sorted input")
	}
	// Confirm that NewMulti does not sort the input
	unsorted := crypto.NewMulti(ms2...)
	if slices.IsSortedFunc(unsorted, cmp) {
		t.Error("NewMulti output is sorted for unsorted input")
	}
	// NewMultiSorted should always return sorted output even for unsorted input
	sorted2 := crypto.NewMultiSorted(ms2...)
	if !slices.IsSortedFunc(sorted2, cmp) {
		t.Error("NewMultiSorted output is not sorted for unsorted input")
	}
	// NewMultiSorted should return sorted output when given sorted input
	sorted3 := crypto.NewMultiSorted(ms1...)
	if !slices.IsSortedFunc(sorted3, cmp) {
		t.Error("NewMultiSorted output is not sorted for sorted input")
	}
}

func TestMultiSignatureLen(t *testing.T) {
	tests := []struct {
		name       string
		numSigners int
	}{
		{"Empty", 0},
		{"Multiple", 4},
		{"Multiple", 10},
	}
	for _, tt := range tests {
		t.Run(test.Name(tt.name, "n", tt.numSigners), func(t *testing.T) {
			s := createMultiSignature(t, tt.numSigners)
			if got := s.Len(); got != tt.numSigners {
				t.Errorf("Len() = %v, want %v", got, tt.numSigners)
			}
		})
	}
}

func TestMultiSignatureContains(t *testing.T) {
	tests := []struct {
		name       string
		numSigners int
		id         hotstuff.ID
		want       bool
	}{
		{"ContainsNoSigners", 0, 0, false}, // ID starts from 1
		{"ContainsNoSigners", 0, 1, false},
		{"ContainsNoSigners", 0, 2, false},
		{"ContainsNoSigners", 0, 3, false},
		{"ContainsFirst", 4, 0, false}, // ID starts from 1
		{"ContainsFirst", 4, 1, true},
		{"ContainsMiddle", 4, 2, true},
		{"ContainsMiddle", 4, 3, true},
		{"ContainsLast", 4, 4, true},
		{"NotContains", 4, 5, false},
		{"NotContains", 4, 6, false},
		{"NotContainsLarge", 4, 100, false},
	}
	for _, tt := range tests {
		t.Run(test.Name(tt.name, "n", tt.numSigners, "id", tt.id), func(t *testing.T) {
			s := createMultiSignature(t, tt.numSigners)
			if got := s.Contains(tt.id); got != tt.want {
				t.Errorf("Contains(%v) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestMultiSignatureForEach(t *testing.T) {
	tests := []struct {
		name       string
		numSigners int
	}{
		{"Empty", 0},
		{"Small", 4},
		{"Large", 10},
		{"Big", 100},
	}
	for _, tt := range tests {
		t.Run(test.Name(tt.name, "n", tt.numSigners), func(t *testing.T) {
			s := createMultiSignature(t, tt.numSigners)
			visited := make(map[hotstuff.ID]bool)
			s.ForEach(func(id hotstuff.ID) { visited[id] = true })
			if len(visited) != tt.numSigners {
				t.Errorf("ForEach() visited %v elements, want %v", len(visited), tt.numSigners)
			}
			for i := 1; i <= tt.numSigners; i++ {
				if !visited[hotstuff.ID(i)] {
					t.Errorf("ForEach() did not visit ID %v", i)
				}
			}
		})
	}
}

func TestMultiSignatureRangeWhile(t *testing.T) {
	tests := []struct {
		name       string
		numSigners int
		stopAfter  int
		wantVisits int // expected number of visits to the RangeWhile function
	}{
		{name: "NoSigners", numSigners: 0, stopAfter: 0, wantVisits: 0},
		{name: "Partial", numSigners: 5, stopAfter: 0, wantVisits: 1},
		{name: "Partial", numSigners: 5, stopAfter: 1, wantVisits: 2},
		{name: "Partial", numSigners: 5, stopAfter: 2, wantVisits: 3},
		{name: "Partial", numSigners: 5, stopAfter: 3, wantVisits: 4},
		{name: "Exhausted", numSigners: 5, stopAfter: 4, wantVisits: 5},
		{name: "Exhausted", numSigners: 5, stopAfter: 10, wantVisits: 5},
	}
	for _, tt := range tests {
		t.Run(test.Name(tt.name, "n", tt.numSigners, "stopAfter", tt.stopAfter, "wantVisits", tt.wantVisits), func(t *testing.T) {
			s := createMultiSignature(t, tt.numSigners)
			count := 0
			s.RangeWhile(func(_ hotstuff.ID) bool {
				count++
				return count <= tt.stopAfter
			})
			if count != tt.wantVisits {
				t.Errorf("RangeWhile() visited %v elements, want at most %v", count, tt.wantVisits)
			}
		})
	}
}

func TestMultiSignatureToBytes(t *testing.T) {
	tests := []struct {
		name       string
		numSigners int
	}{
		{"Empty", 0},
		{"Multiple", 4},
		{"Multiple", 10},
	}
	for _, tt := range tests {
		t.Run(test.Name(tt.name, "n", tt.numSigners), func(t *testing.T) {
			s := createMultiSignature(t, tt.numSigners)
			b1 := s.ToBytes()
			b2 := s.ToBytes()
			if tt.numSigners == 0 {
				if len(b1) != 0 {
					t.Errorf("ToBytes() = %v bytes, want 0", len(b1))
				}
				return
			}
			if !bytes.Equal(b1, b2) {
				t.Errorf("ToBytes() is not deterministic")
			}
			if len(b1) == 0 {
				t.Errorf("ToBytes() returned empty bytes for non-empty signature")
			}
		})
	}
}

func TestMultiSignatureParticipants(t *testing.T) {
	tests := []struct {
		name       string
		numSigners int
	}{
		{"Empty", 0},
		{"Multiple", 4},
		{"Multiple", 10},
	}
	for _, tt := range tests {
		t.Run(test.Name(tt.name, "n", tt.numSigners), func(t *testing.T) {
			participants := createMultiSignature(t, tt.numSigners).Participants()
			if participants.Len() != tt.numSigners {
				t.Errorf("Participants().Len() = %v, want %v", participants.Len(), tt.numSigners)
			}
			for i := 1; i <= tt.numSigners; i++ {
				if !participants.Contains(hotstuff.ID(i)) {
					t.Errorf("Participants() does not contain ID %v", i)
				}
			}
		})
	}
}

func BenchmarkForEach(b *testing.B) {
	for _, n := range []int{10, 100, 1000} {
		b.Run(test.Name("n", n), func(b *testing.B) {
			ms := createMultiSignature(b, n)
			b.ResetTimer()
			for b.Loop() {
				count := 0
				ms.ForEach(func(_ hotstuff.ID) {
					count++
				})
			}
		})
	}
}

func BenchmarkToBytes(b *testing.B) {
	for _, n := range []int{10, 100, 1000} {
		b.Run(test.Name("n", n), func(b *testing.B) {
			ms := createMultiSignature(b, n)
			b.ResetTimer()
			for b.Loop() {
				_ = ms.ToBytes()
			}
		})
	}
}
