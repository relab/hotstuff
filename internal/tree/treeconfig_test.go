package tree

import (
	"fmt"
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
		{configurationSize: 43, id: 1, branchFactor: 5, wantHeight: 4},
		{configurationSize: 43, id: 1, branchFactor: 4, wantHeight: 4},
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
	subTreeReplicas   []hotstuff.ID
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
		gotChildren := tree.ReplicaChildren()
		sort.Slice(gotChildren, func(i, j int) bool { return gotChildren[i] < gotChildren[j] })
		if len(gotChildren) != len(test.children) || !slices.Equal(gotChildren, test.children) {
			t.Errorf("Expected %v, got %v", test.children, tree.ReplicaChildren())
		}
		subTree := tree.SubTree()
		sort.Slice(subTree, func(i, j int) bool { return subTree[i] < subTree[j] })
		if len(subTree) != len(test.subTreeReplicas) ||
			!slices.Equal(subTree, test.subTreeReplicas) {
			t.Errorf("Expected %v, got %v", test.subTreeReplicas, tree.SubTree())
		}
		if parent, ok := tree.Parent(); ok {
			if parent != test.parent {
				t.Errorf("Expected %d, got %d", test.parent, parent)
			}
		}
		if tree.IsRoot(test.id) != test.isRoot {
			t.Errorf("Expected %t, got %t", test.isRoot, tree.IsRoot(test.id))
		}
		if tree.ReplicaHeight() != test.replicaHeight {
			t.Errorf("Expected %d, got %d", test.replicaHeight, tree.ReplicaHeight())
		}
		gotPeers := tree.PeersOf(test.id)
		sort.Slice(gotPeers, func(i, j int) bool { return gotPeers[i] < gotPeers[j] })
		if len(gotPeers) != len(test.peers) || !slices.Equal(gotPeers, test.peers) {
			t.Errorf("Expected %v, got %v", test.peers, tree.PeersOf(test.id))
		}
	}
}

func BenchmarkReplicaChildren(b *testing.B) {
	benchmarks := []struct {
		size int
		bf   int
	}{
		{size: 10, bf: 2},
		{size: 21, bf: 4},
		{size: 111, bf: 10},
		{size: 211, bf: 14},
		{size: 421, bf: 20},
	}
	for _, bm := range benchmarks {
		b.Run(fmt.Sprintf("size=%d/bf=%d", bm.size, bm.bf), func(b *testing.B) {
			ids := make([]hotstuff.ID, bm.size)
			for i := 0; i < bm.size; i++ {
				ids[i] = hotstuff.ID(i + 1)
			}
			tree := CreateTree(1, bm.bf, ids)
			// replace `for range b.N` with `for b.Loop()` when go 1.24 released (in release candidate as of writing)
			// for b.Loop() {
			for range b.N {
				tree.ReplicaChildren()
			}
		})
	}
}
