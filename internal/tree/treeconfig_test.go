package tree

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
		ids := make([]hotstuff.ID, test.configurationSize)
		for i := 0; i < test.configurationSize; i++ {
			ids[i] = hotstuff.ID(i + 1)
		}
		tree := CreateTree(test.id, test.branchFactor, ids)
		if tree.TreeHeight() != test.wantHeight {
			t.Errorf("CreateTree(%d, %d, %d).GetTreeHeight() = %d, want %d",
				test.configurationSize, test.id, test.branchFactor, tree.TreeHeight(), test.wantHeight)
		}
	}
}

func TestCreateTreeNegativeBF(t *testing.T) {
	defer func() { _ = recover() }()
	ids := []hotstuff.ID{1, 2, 3, 4, 5}
	tree := CreateTree(1, -1, ids)
	t.Errorf("CreateTree should panic, got %v", tree)
}

func TestCreateTreeInvalidID(t *testing.T) {
	defer func() { _ = recover() }()
	ids := []hotstuff.ID{1, 2, 3, 4, 5}
	tree := CreateTree(10, 2, ids)
	t.Errorf("CreateTree should panic, got %v", tree)
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
		ids := make([]hotstuff.ID, test.configurationSize)
		for i := 0; i < test.configurationSize; i++ {
			ids[i] = hotstuff.ID(i + 1)
		}
		tree := CreateTree(test.id, test.branchFactor, ids)
		if tree.TreeHeight() != test.height {
			t.Errorf("Expected height %d, got %d", test.height, tree.TreeHeight())
		}
		gotChildren := tree.ChildrenOf()
		sort.Slice(gotChildren, func(i, j int) bool { return gotChildren[i] < gotChildren[j] })
		if len(gotChildren) != len(test.children) || !slices.Equal(gotChildren, test.children) {
			t.Errorf("Expected %v, got %v", test.children, tree.ChildrenOf())
		}
		subTree := tree.SubTree()
		sort.Slice(subTree, func(i, j int) bool { return subTree[i] < subTree[j] })
		if len(subTree) != len(test.subTreeNodes) ||
			!slices.Equal(subTree, test.subTreeNodes) {
			t.Errorf("Expected %v, got %v", test.subTreeNodes, tree.SubTree())
		}
		if parent, ok := tree.Parent(); ok {
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
		gotPeers := tree.PeersOf(test.id)
		sort.Slice(gotPeers, func(i, j int) bool { return gotPeers[i] < gotPeers[j] })
		if len(gotPeers) != len(test.peers) || !slices.Equal(gotPeers, test.peers) {
			t.Errorf("Expected %v, got %v", test.peers, tree.PeersOf(test.id))
		}
	}
}

func benchmarkGetChildren(size int, bf int, b *testing.B) {
	ids := make([]hotstuff.ID, size)
	for i := 0; i < size; i++ {
		ids[i] = hotstuff.ID(i + 1)
	}
	tree := CreateTree(1, bf, ids)
	for i := 0; i < b.N; i++ {
		tree.ChildrenOf()
	}
}

func BenchmarkGetChildren10(b *testing.B) {
	benchmarkGetChildren(10, 2, b)
}

func BenchmarkGetChildren21(b *testing.B) {
	benchmarkGetChildren(21, 4, b)
}

func BenchmarkGetChildren111(b *testing.B) {
	benchmarkGetChildren(111, 10, b)
}

func BenchmarkGetChildren211(b *testing.B) {
	benchmarkGetChildren(211, 14, b)
}

func BenchmarkGetChildren421(b *testing.B) {
	benchmarkGetChildren(421, 20, b)
}
