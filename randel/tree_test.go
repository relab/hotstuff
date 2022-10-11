package randel

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/relab/hotstuff"
)

type result struct {
	id  hotstuff.ID
	ret bool
}

func TestGetParent(t *testing.T) {
	expectedParents := map[int]result{
		0:  {0, false},
		1:  {0, true},
		2:  {0, true},
		3:  {0, true},
		4:  {1, true},
		5:  {1, true},
		6:  {1, true},
		7:  {2, true},
		8:  {2, true},
		9:  {2, true},
		10: {3, true},
		11: {3, true},
		12: {3, true},
	}
	configLength := 13
	ids := make(map[hotstuff.ID]int)
	for i := 0; i < configLength; i++ {
		ids[hotstuff.ID(i)] = i
	}
	for i := 0; i < configLength; i++ {
		tree := CreateTree(configLength, hotstuff.ID(i))
		tree.InitializeWithPIDs(ids)
		gotID, gotRes := tree.GetParent()
		expID := expectedParents[i]
		if gotID != expID.id || gotRes != expID.ret {
			t.Errorf("error in GetParent(%d) expected Parent is %d and Got Parent is %d\n",
				i, expID.id, gotID)
		}
	}
}

func TestGetChildren(t *testing.T) {
	expectedChildren := map[int][]hotstuff.ID{
		0: {hotstuff.ID(1), hotstuff.ID(2), hotstuff.ID(3)},
		1: {hotstuff.ID(4), hotstuff.ID(5), hotstuff.ID(6)},
		2: {hotstuff.ID(7), hotstuff.ID(8), hotstuff.ID(9)},
		3: {hotstuff.ID(10)},
		4: {},
	}
	configLength := 11
	ids := make(map[hotstuff.ID]int)
	for i := 0; i < configLength; i++ {
		ids[hotstuff.ID(i)] = i
	}
	for i := 0; i < 5; i++ {
		tree := CreateTree(configLength, hotstuff.ID(i))
		tree.InitializeWithPIDs(ids)
		gotChild := tree.GetChildren()
		if diff := cmp.Diff(gotChild, expectedChildren[i]); diff != "" {
			t.Errorf("error in GetChildren(%d), difference is %s\n", i, diff)
		}
	}
}

func TestGetGrandParent(t *testing.T) {
	expectedGP := map[int]result{
		0: {0, false},
		1: {0, false},
		2: {0, false},
		3: {0, false},
		4: {0, true},
		5: {0, true},
	}
	configLength := 10
	ids := make(map[hotstuff.ID]int)
	for i := 0; i < configLength; i++ {
		ids[hotstuff.ID(i)] = i
	}
	for i := 0; i < 6; i++ {
		tree := CreateTree(configLength, hotstuff.ID(i))
		tree.InitializeWithPIDs(ids)
		gotGP, res := tree.GetGrandParent()
		expectedRes := expectedGP[i]

		if gotGP != expectedRes.id || res != expectedRes.ret {
			t.Errorf("error in GetGrandParent(%d), expected %d got %d \n", i, gotGP, expectedRes.id)
		}
	}
}
