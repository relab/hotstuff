package trees

import (
	"slices"
	"sort"
	"testing"

	"github.com/relab/hotstuff"
)

func TestCreateTree(t *testing.T) {
	tests := []struct {
		configurationSize int
		id                hotstuff.ID
		branchFactor      int
		wantHeight        int
	}{
		{configurationSize: 10, id: 1, branchFactor: 2, wantHeight: 4},
		{configurationSize: 21, id: 1, branchFactor: 4, wantHeight: 3},
		{configurationSize: 21, id: 1, branchFactor: 3, wantHeight: 4},
		{configurationSize: 111, id: 1, branchFactor: 10, wantHeight: 3},
		{configurationSize: 111, id: 1, branchFactor: 3, wantHeight: 5},
	}
	for _, test := range tests {
		tree := CreateTree(test.configurationSize, test.id, test.branchFactor)
		if tree.GetTreeHeight() != test.wantHeight {
			t.Errorf("CreateTree(%d, %d, %d).GetTreeHeight() = %d, want %d",
				test.configurationSize, test.id, test.branchFactor, tree.GetTreeHeight(), test.wantHeight)
		}
	}
}

func TestTreeWithNegativeCases(t *testing.T) {
	tree := CreateTree(10, 1, 2)
	if tree.GetTreeHeight() != 4 {
		t.Errorf("Expected height 4, got %d", tree.GetTreeHeight())
	}
	if len(tree.GetChildren()) != 0 {
		t.Errorf("Expected nil, got %v", tree.GetChildren())
	}
	if len(tree.GetSubTreeNodes()) != 0 {
		t.Errorf("Expected nil, got %v", tree.GetSubTreeNodes())
	}
	if _, ok := tree.GetParent(); ok {
		t.Errorf("Expected false, got true")
	}
	tree = CreateTree(-1, 1, 2)
	if tree != nil {
		t.Errorf("Expected nil, got %v", tree)
	}
	ids := []hotstuff.ID{1, 2, 3, 3, 4, 5, 6, 7, 8, 9}
	tree = CreateTree(10, 1, 2)
	err := tree.InitializeWithPIDs(ids)
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
	ids = []hotstuff.ID{1, 2, 3, 4, 5, 6, 7, 8, 9}
	err = tree.InitializeWithPIDs(ids)
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
}

type treeConfigTest struct {
	configurationSize int
	id                hotstuff.ID
	branchFactor      int
	height            int
	children          []hotstuff.ID
	subTreeNodes      []hotstuff.ID
	parent            hotstuff.ID
	isRoot            bool
	replicaHeight     int
	peers             []hotstuff.ID
}

func TestTreeAPIWithInitializeWithPIDs(t *testing.T) {
	treeConfigTestData := []treeConfigTest{
		{10, 1, 2, 4, []hotstuff.ID{2, 3}, []hotstuff.ID{2, 3, 4, 5, 6, 7, 8, 9, 10}, 1, true, 4, []hotstuff.ID{}},
		{10, 5, 2, 4, []hotstuff.ID{10}, []hotstuff.ID{10}, 2, false, 2, []hotstuff.ID{4, 5}},
		{10, 2, 2, 4, []hotstuff.ID{4, 5}, []hotstuff.ID{4, 5, 8, 9, 10}, 1, false, 3, []hotstuff.ID{2, 3}},
		{10, 3, 2, 4, []hotstuff.ID{6, 7}, []hotstuff.ID{6, 7}, 1, false, 3, []hotstuff.ID{2, 3}},
		{10, 4, 2, 4, []hotstuff.ID{8, 9}, []hotstuff.ID{8, 9}, 2, false, 2, []hotstuff.ID{4, 5}},
		{21, 1, 4, 3, []hotstuff.ID{2, 3, 4, 5}, []hotstuff.ID{
			2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21,
		}, 1, true, 3, []hotstuff.ID{}},
		{21, 1, 3, 4, []hotstuff.ID{2, 3, 4}, []hotstuff.ID{
			2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21,
		}, 1, true, 4, []hotstuff.ID{}},
		{21, 2, 4, 3, []hotstuff.ID{6, 7, 8, 9}, []hotstuff.ID{6, 7, 8, 9}, 1, false, 2, []hotstuff.ID{2, 3, 4, 5}},
		{21, 2, 3, 4, []hotstuff.ID{5, 6, 7}, []hotstuff.ID{5, 6, 7, 14, 15, 16, 17, 18, 19, 20, 21}, 1, false, 3, []hotstuff.ID{2, 3, 4}},
		{21, 9, 3, 4, []hotstuff.ID{}, []hotstuff.ID{}, 3, false, 2, []hotstuff.ID{8, 9, 10}},
		{21, 7, 3, 4, []hotstuff.ID{20, 21}, []hotstuff.ID{20, 21}, 2, false, 2, []hotstuff.ID{5, 6, 7}},
		{21, 3, 4, 3, []hotstuff.ID{10, 11, 12, 13}, []hotstuff.ID{10, 11, 12, 13}, 1, false, 2, []hotstuff.ID{2, 3, 4, 5}},
		{21, 10, 4, 3, []hotstuff.ID{}, []hotstuff.ID{}, 3, false, 1, []hotstuff.ID{10, 11, 12, 13}},
		{21, 15, 4, 3, []hotstuff.ID{}, []hotstuff.ID{}, 4, false, 1, []hotstuff.ID{14, 15, 16, 17}},
		{21, 20, 4, 3, []hotstuff.ID{}, []hotstuff.ID{}, 5, false, 1, []hotstuff.ID{18, 19, 20, 21}},
		{21, 5, 4, 3, []hotstuff.ID{18, 19, 20, 21}, []hotstuff.ID{18, 19, 20, 21}, 1, false, 2, []hotstuff.ID{2, 3, 4, 5}},
	}
	for _, test := range treeConfigTestData {
		tree := CreateTree(test.configurationSize, test.id, test.branchFactor)
		ids := make([]hotstuff.ID, test.configurationSize)
		for i := 0; i < test.configurationSize; i++ {
			ids[i] = hotstuff.ID(i + 1)
		}
		if err := tree.InitializeWithPIDs(ids); err != nil {
			t.Errorf("Expected nil, got %v", err)
		}
		if tree.GetTreeHeight() != test.height {
			t.Errorf("Expected height %d, got %d", test.height, tree.GetTreeHeight())
		}
		gotChildren := tree.GetChildren()
		sort.Slice(gotChildren, func(i, j int) bool { return gotChildren[i] < gotChildren[j] })
		if len(gotChildren) != len(test.children) || !slices.Equal(gotChildren, test.children) {
			t.Errorf("Expected %v, got %v", test.children, tree.GetChildren())
		}
		subTree := tree.GetSubTreeNodes()
		sort.Slice(subTree, func(i, j int) bool { return subTree[i] < subTree[j] })
		if len(subTree) != len(test.subTreeNodes) ||
			!slices.Equal(subTree, test.subTreeNodes) {
			t.Errorf("Expected %v, got %v", test.subTreeNodes, tree.GetSubTreeNodes())
		}
		if parent, ok := tree.GetParent(); ok {
			if parent != test.parent {
				t.Errorf("Expected %d, got %d", test.parent, parent)
			}
		}
		if tree.IsRoot(test.id) != test.isRoot {
			t.Errorf("Expected %t, got %t", test.isRoot, tree.IsRoot(test.id))
		}
		if tree.GetHeight() != test.replicaHeight {
			t.Errorf("Expected %d, got %d", test.replicaHeight, tree.GetHeight())
		}
		gotPeers := tree.GetPeers(test.id)
		sort.Slice(gotPeers, func(i, j int) bool { return gotPeers[i] < gotPeers[j] })
		if len(gotPeers) != len(test.peers) || !slices.Equal(gotPeers, test.peers) {
			t.Errorf("Expected %v, got %v", test.peers, tree.GetPeers(test.id))
		}
	}
}
