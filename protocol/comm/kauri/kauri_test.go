package kauri_test

import (
	"slices"
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/test"
	"github.com/relab/hotstuff/protocol/comm/kauri"
	"github.com/relab/hotstuff/security/crypto"
)

func TestSubTree(t *testing.T) {
	testData := []struct {
		a    []hotstuff.ID
		b    []hotstuff.ID
		want bool
	}{
		{[]hotstuff.ID{1, 2, 3, 4, 5}, []hotstuff.ID{1, 2, 3, 4, 5}, true},
		{[]hotstuff.ID{1, 2, 3, 4, 5}, []hotstuff.ID{1, 2, 3, 4}, false},
		{[]hotstuff.ID{1, 2, 3, 4, 5}, []hotstuff.ID{1, 2, 3, 4, 6}, false},
		{[]hotstuff.ID{1, 2, 3, 4, 5}, []hotstuff.ID{1, 2, 3, 4, 5, 6}, true},
		{nil, nil, true},
	}
	for _, test := range testData {
		if kauri.IsSubSet(test.a, test.b) != test.want {
			t.Errorf("SubTree(%v, %v) = %t; want %t", test.a, test.b, !test.want, test.want)
		}
	}
}

func BenchmarkSubTree(b *testing.B) {
	benchData := []struct {
		size  int
		start int // start index for the subset
	}{
		{10, 8},  // subset size 2
		{10, 5},  // subset size 5
		{40, 30}, // subset size 10
		{40, 20}, // subset size 20
		{80, 50}, // subset size 30
		{80, 40}, // subset size 40
		{80, 30}, // subset size 50
		{80, 20}, // subset size 60
		{80, 10}, // subset size 70
	}
	for _, data := range benchData {
		set := make([]hotstuff.ID, data.size)
		for i := range set {
			set[i] = hotstuff.ID(i + 1)
		}
		subSet := set[data.start:]
		b.Run(test.Name("SliceSet", "size", len(set), "q", len(subSet)), func(b *testing.B) {
			for b.Loop() {
				kauri.IsSubSet(subSet, set)
			}
		})
		b.Run(test.Name("MapSet__", "size", len(set), "q", len(subSet)), func(b *testing.B) {
			for b.Loop() {
				isSubSetMap(subSet, set)
			}
		})
	}
}

func isSubSetMap(a, b []hotstuff.ID) bool {
	set := make(map[hotstuff.ID]struct{}, len(b))
	for _, id := range b {
		set[id] = struct{}{}
	}
	for _, id := range a {
		if _, ok := set[id]; !ok {
			return false
		}
	}
	return true
}

func TestCanMerge(t *testing.T) {
	testData := []struct {
		a       []hotstuff.ID
		b       []hotstuff.ID
		wantErr bool
	}{
		{[]hotstuff.ID{1, 2, 3, 4, 5}, []hotstuff.ID{1, 2, 3, 4, 5}, true},
		{[]hotstuff.ID{1, 2, 3, 4, 5}, []hotstuff.ID{1, 2, 3, 4, 5, 6}, true},
		{[]hotstuff.ID{1, 2}, []hotstuff.ID{3, 4}, false},
		{[]hotstuff.ID{1, 2, 3, 4, 5}, []hotstuff.ID{1, 2}, true},
		{[]hotstuff.ID{3, 4, 5}, []hotstuff.ID{1, 2}, false},
		{nil, nil, false},
	}
	for _, test := range testData {
		overlaps := hasOverlap(test.a, test.b)
		if overlaps && !test.wantErr {
			t.Errorf("hasOverlap(%v, %v) = true, want false", test.a, test.b)
		}
		if !overlaps && test.wantErr {
			t.Errorf("hasOverlap(%v, %v) = false, want true", test.a, test.b)
		}
		a := make(crypto.Multi[*crypto.ECDSASignature])
		for _, id := range test.a {
			a[id] = &crypto.ECDSASignature{}
		}
		b := make(crypto.Multi[*crypto.ECDSASignature])
		for _, id := range test.b {
			b[id] = &crypto.ECDSASignature{}
		}
		err := kauri.CanMergeContributions(a, b)
		if err != nil && !test.wantErr {
			t.Errorf("canMergeContributions(%v, %v) got error and no error is expected", a, b)
		}
		if err == nil && test.wantErr {
			t.Errorf("canMergeContributions(%v, %v) succeeded and error is expected", a, b)
		}
	}
	err := kauri.CanMergeContributions(nil, nil)
	if err == nil {
		t.Errorf("canMergeContributions(nil, nil) succeeded and error is expected")
	}
}

func BenchmarkHasOverlap(b *testing.B) {
	benchData := []struct {
		size  int
		start int // start index for the subset
	}{
		{200, 200}, // subset size 0
		{200, 150}, // subset size 50
		{200, 100}, // subset size 100
		{200, 50},  // subset size 150
		{200, 0},   // subset size 200
		{500, 400}, // subset size 100
		{500, 300}, // subset size 200
	}
	for _, data := range benchData {
		set := make([]hotstuff.ID, data.size)
		for i := range set {
			set[i] = hotstuff.ID(i + 1)
		}
		subSet := set[data.start:]

		p := make(crypto.Multi[*crypto.ECDSASignature])
		for _, id := range set {
			p[id] = &crypto.ECDSASignature{}
		}
		q := make(crypto.Multi[*crypto.ECDSASignature])
		for _, id := range subSet {
			q[id] = &crypto.ECDSASignature{}
		}
		b.Run(test.Name("HasOverlap", "size", len(set), "q", len(subSet)), func(b *testing.B) {
			for b.Loop() {
				hasOverlap(subSet, set)
			}
		})
		b.Run(test.Name("CanMerge", "size", len(set), "q", len(subSet)), func(b *testing.B) {
			for b.Loop() {
				// intentionally discarding error since it's not relevant in the benchmark
				_ = kauri.CanMergeContributions(p, q)
			}
		})
	}
}

// hasOverlap returns true if there is any overlap between a and b.
func hasOverlap(a, b []hotstuff.ID) bool {
	for _, id := range a {
		if slices.Contains(b, id) {
			return true
		}
	}
	return false
}
