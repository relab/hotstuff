package crypto

import (
	"math/rand"
	"testing"

	"github.com/relab/hotstuff"
)

// TestBitfieldAdd checks that the bitfield can extend itself to fit larger IDs.
func TestBitfieldAdd(t *testing.T) {
	testCases := []struct {
		id  hotstuff.ID
		len int
	}{{1, 1}, {9, 2}, {17, 3}}

	var bm Bitfield

	// initial length should be 0
	if len(bm.data) != 0 {
		t.Errorf("Unexpected length: got: %v, want: %v", len(bm.data), 0)
	}

	for _, testCase := range testCases {
		bm.Add(testCase.id)
		if len(bm.data) != testCase.len {
			t.Errorf("Unexpected length: got: %v, want: %v", len(bm.data), testCase.len)
		}
	}
}

func TestBitfieldContains(t *testing.T) {
	random := hotstuff.ID(rand.Intn(254)) + 2 // should not be 0 or 1
	testCases := []hotstuff.ID{1, random, random + 1}

	var bm Bitfield

	// first check that the bitfield returns false for all testCases
	for _, testCase := range testCases {
		if bm.Contains(testCase) {
			t.Errorf("Wrong result for id %d: got: true, want: false", testCase)
		}
	}

	// add all test cases
	for _, testCase := range testCases {
		bm.Add(testCase)
	}

	// now check that the bitfield contains the test cases
	for _, testCase := range testCases {
		if !bm.Contains(testCase) {
			t.Errorf("Wrong result for id %d: got: false, want: true", testCase)
		}
	}
}

func TestBitfieldForEach(t *testing.T) {
	random := hotstuff.ID(rand.Intn(254)) + 2 // should not be 0 or 1
	testCases := []hotstuff.ID{1, random, random + 1}

	var bm Bitfield

	// first check that the bitfield is empty
	count := 0
	bm.ForEach(func(i hotstuff.ID) {
		count++
	})

	if count != 0 {
		t.Error("bitfield was not empty")
	}

	// add all test cases
	for _, testCase := range testCases {
		bm.Add(testCase)
	}

	// now check that the bitfield contains the test cases
	var got []hotstuff.ID
	bm.ForEach(func(i hotstuff.ID) {
		got = append(got, i)
	})

	if len(got) != len(testCases) {
		t.Fatal("ForEach gave the wrong number of IDs")
	}

	for i := range got {
		if got[i] != testCases[i] {
			t.Errorf("got: %d, want: %d", got[i], testCases[i])
		}
	}
}
