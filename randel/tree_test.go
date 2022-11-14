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

func TestGetPeers(t *testing.T) {
	expectedPeers := map[int][]hotstuff.ID{
		0:  {},
		1:  {1, 2, 3, 4},
		2:  {1, 2, 3, 4},
		3:  {1, 2, 3, 4},
		4:  {1, 2, 3, 4},
		5:  {5, 6, 7, 8},
		6:  {5, 6, 7, 8},
		7:  {5, 6, 7, 8},
		8:  {5, 6, 7, 8},
		9:  {9, 10, 11, 12},
		10: {9, 10, 11, 12},
		11: {9, 10, 11, 12},
		12: {9, 10, 11, 12},
		13: {13},
	}
	configLength := 14
	ids := make(map[hotstuff.ID]int)
	for i := 0; i < configLength; i++ {
		ids[hotstuff.ID(i)] = i
	}
	for i := 0; i < configLength; i++ {
		tree := CreateTree(configLength, hotstuff.ID(i))
		tree.InitializeWithPIDs(ids)
		gotRes := tree.GetPeers(hotstuff.ID(i))
		if diff := cmp.Diff(gotRes, expectedPeers[i]); diff != "" {
			t.Errorf("error in GetChildren(%d), difference is %s\n", i, diff)
		}
	}
}

func TestSubTree(t *testing.T) {
	expectedSubTree := map[int][]hotstuff.ID{
		0: {1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13},
		1: {5, 6, 7, 8},
		2: {9, 10, 11, 12},
		3: {13},
		4: {},
	}
	configLength := 14
	ids := make(map[hotstuff.ID]int)
	for i := 0; i < configLength; i++ {
		ids[hotstuff.ID(i)] = i
	}
	for i := 0; i < MAX_CHILD+1; i++ {
		tree := CreateTree(configLength, hotstuff.ID(i))
		tree.InitializeWithPIDs(ids)
		gotRes := tree.GetSubTreeNodes(hotstuff.ID(i))
		if diff := cmp.Diff(gotRes, expectedSubTree[i]); diff != "" {
			t.Errorf("error in GetChildren(%d), difference is %s\n", i, diff)
		}
	}
}

func TestGetHeight(t *testing.T) {
	expectedHeight := map[int]int{
		0:  3,
		1:  2,
		2:  2,
		3:  2,
		4:  2,
		5:  1,
		6:  1,
		7:  1,
		8:  1,
		9:  1,
		10: 1,
		11: 1,
		12: 1,
		13: 1,
	}
	configLength := 14
	ids := make(map[hotstuff.ID]int)
	for i := 0; i < configLength; i++ {
		ids[hotstuff.ID(i)] = i
	}
	for i := 0; i < configLength; i++ {
		tree := CreateTree(configLength, hotstuff.ID(i))
		tree.InitializeWithPIDs(ids)
		gotRes := tree.GetHeight(hotstuff.ID(i))
		if gotRes != expectedHeight[i] {
			t.Errorf("error in GetHeight(%d), expected height %d got height %d\n", i, expectedHeight[i], gotRes)
		}
	}
}
