package kauri

import (
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/crypto"
	"github.com/relab/hotstuff/crypto/ecdsa"
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
		if isSubSet(test.a, test.b) != test.want {
			t.Errorf("SubTree(%v, %v) = %t; want %t", test.a, test.b, !test.want, test.want)
		}
	}
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
		a := make(crypto.Multi[*ecdsa.Signature])
		for _, id := range test.a {
			a[id] = &ecdsa.Signature{}
		}
		b := make(crypto.Multi[*ecdsa.Signature])
		for _, id := range test.b {
			b[id] = &ecdsa.Signature{}
		}
		err := canMergeContributions(a, b)
		if err != nil && !test.wantErr {
			t.Errorf("canMergeContributions(%v, %v) got error and no error is expected", a, b)
		}
		if err == nil && test.wantErr {
			t.Errorf("canMergeContributions(%v, %v) succeeded and error is expected", a, b)
		}
	}
	err := canMergeContributions(nil, nil)
	if err == nil {
		t.Errorf("canMergeContributions(nil, nil) succeeded and error is expected")
	}
}
