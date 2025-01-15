package kauri

import (
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/crypto"
	"github.com/relab/hotstuff/crypto/ecdsa"
)

func TestSubTree(t *testing.T) {
	subTreeTestData := []struct {
		test_a   []hotstuff.ID
		test_b   []hotstuff.ID
		expected bool
	}{
		{[]hotstuff.ID{1, 2, 3, 4, 5}, []hotstuff.ID{1, 2, 3, 4, 5}, true},
		{[]hotstuff.ID{1, 2, 3, 4, 5}, []hotstuff.ID{1, 2, 3, 4}, false},
		{[]hotstuff.ID{1, 2, 3, 4, 5}, []hotstuff.ID{1, 2, 3, 4, 6}, false},
		{[]hotstuff.ID{1, 2, 3, 4, 5}, []hotstuff.ID{1, 2, 3, 4, 5, 6}, true},
		{nil, nil, true},
	}
	for _, test := range subTreeTestData {
		if isSubSet(test.test_a, test.test_b) != test.expected {
			t.Errorf("SubTree(%v, %v) = %v; want %v", test.test_a, test.test_b, !test.expected, test.expected)
		}
	}
}

func TestCanMerge(t *testing.T) {
	mergeTestData := []struct {
		test_a   []hotstuff.ID
		test_b   []hotstuff.ID
		expected bool
	}{
		{[]hotstuff.ID{1, 2, 3, 4, 5}, []hotstuff.ID{1, 2, 3, 4, 5}, true},
		{[]hotstuff.ID{1, 2, 3, 4, 5}, []hotstuff.ID{1, 2, 3, 4, 5, 6}, true},
		{[]hotstuff.ID{1, 2}, []hotstuff.ID{3, 4}, false},
		{[]hotstuff.ID{1, 2, 3, 4, 5}, []hotstuff.ID{1, 2}, true},
		{[]hotstuff.ID{3, 4, 5}, []hotstuff.ID{1, 2}, false},
		{nil, nil, false},
	}
	for _, test := range mergeTestData {
		a := make(crypto.Multi[*ecdsa.Signature])
		for _, id := range test.test_a {
			a[id] = &ecdsa.Signature{}
		}
		b := make(crypto.Multi[*ecdsa.Signature])
		for _, id := range test.test_b {
			b[id] = &ecdsa.Signature{}
		}
		err := canMergeContributions(a, b)
		if err != nil && !test.expected {
			t.Errorf("canMergeContributions(%v, %v) got error and no error is expected", a, b)
		}
		if err == nil && test.expected {
			t.Errorf("canMergeContributions(%v, %v) succeeded and error is expected", a, b)
		}
	}
	err := canMergeContributions(nil, nil)
	if err == nil {
		t.Errorf("canMergeContributions(nil, nil) succeeded and error is expected")
	}
}
