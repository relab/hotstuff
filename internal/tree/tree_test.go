package tree

import (
	"fmt"
	"slices"
	"testing"

	"github.com/relab/hotstuff"
)

// replicaIDs returns a slice of hotstuff.IDs from 1 to size.
func replicaIDs(size int) []hotstuff.ID {
	ids := make([]hotstuff.ID, size)
	for i := range size {
		ids[i] = hotstuff.ID(i + 1)
	}
	return ids
}

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
		tree := CreateTree(test.id, test.branchFactor, replicaIDs(test.configurationSize))
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

func TestHeight(t *testing.T) {
	treeTestData := []struct {
		configurationSize int
		id                hotstuff.ID
		branchFactor      int
		wantTreeHeight    int
	}{
		{10, 1, 2, 4},
		{10, 5, 2, 4},
		{10, 2, 2, 4},
		{10, 3, 2, 4},
		{10, 4, 2, 4},
		{21, 1, 4, 3},
		{21, 1, 3, 4},
		{21, 2, 4, 3},
		{21, 2, 3, 4},
		{21, 9, 3, 4},
		{21, 10, 4, 3},
	}
	for _, test := range treeTestData {
		tree := CreateTree(test.id, test.branchFactor, replicaIDs(test.configurationSize))
		gotTreeHeight := tree.TreeHeight()
		if gotTreeHeight != test.wantTreeHeight {
			t.Errorf("TreeHeight() = %d, want %d", gotTreeHeight, test.wantTreeHeight)
		}
	}
}

func TestReplicaChildren(t *testing.T) {
	treeConfigTestData := []struct {
		configurationSize int
		id                hotstuff.ID
		branchFactor      int
		wantChildren      []hotstuff.ID
	}{
		{10, 1, 2, []hotstuff.ID{2, 3}},
		{10, 5, 2, []hotstuff.ID{10}},
		{10, 2, 2, []hotstuff.ID{4, 5}},
		{10, 3, 2, []hotstuff.ID{6, 7}},
		{10, 4, 2, []hotstuff.ID{8, 9}},
		{21, 1, 4, []hotstuff.ID{2, 3, 4, 5}},
		{21, 1, 3, []hotstuff.ID{2, 3, 4}},
		{21, 2, 4, []hotstuff.ID{6, 7, 8, 9}},
		{21, 2, 3, []hotstuff.ID{5, 6, 7}},
		{21, 9, 3, []hotstuff.ID{}},
		{21, 7, 3, []hotstuff.ID{20, 21}},
		{21, 3, 4, []hotstuff.ID{10, 11, 12, 13}},
		{21, 10, 4, []hotstuff.ID{}},
		{21, 15, 4, []hotstuff.ID{}},
		{21, 20, 4, []hotstuff.ID{}},
		{21, 5, 4, []hotstuff.ID{18, 19, 20, 21}},
	}
	for _, test := range treeConfigTestData {
		tree := CreateTree(test.id, test.branchFactor, replicaIDs(test.configurationSize))
		gotChildren := tree.ReplicaChildren()
		slices.Sort(gotChildren)
		if !slices.Equal(gotChildren, test.wantChildren) {
			t.Errorf("ReplicaChildren() = %v, want %v", gotChildren, test.wantChildren)
		}
	}
}

func TestSubTree(t *testing.T) {
	treeConfigTestData := []struct {
		configurationSize int
		id                hotstuff.ID
		branchFactor      int
		wantSubTree       []hotstuff.ID
	}{
		{10, 1, 2, []hotstuff.ID{2, 3, 4, 5, 6, 7, 8, 9, 10}},
		{10, 5, 2, []hotstuff.ID{10}},
		{10, 2, 2, []hotstuff.ID{4, 5, 8, 9, 10}},
		{10, 3, 2, []hotstuff.ID{6, 7}},
		{10, 4, 2, []hotstuff.ID{8, 9}},
		{21, 1, 4, []hotstuff.ID{
			2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21,
		}},
		{21, 1, 3, []hotstuff.ID{
			2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21,
		}},
		{21, 2, 4, []hotstuff.ID{6, 7, 8, 9}},
		{21, 2, 3, []hotstuff.ID{5, 6, 7, 14, 15, 16, 17, 18, 19, 20, 21}},
		{21, 9, 3, []hotstuff.ID{}},
		{21, 7, 3, []hotstuff.ID{20, 21}},
		{21, 3, 4, []hotstuff.ID{10, 11, 12, 13}},
		{21, 10, 4, []hotstuff.ID{}},
		{21, 15, 4, []hotstuff.ID{}},
		{21, 20, 4, []hotstuff.ID{}},
		{21, 5, 4, []hotstuff.ID{18, 19, 20, 21}},
	}
	for _, test := range treeConfigTestData {
		tree := CreateTree(test.id, test.branchFactor, replicaIDs(test.configurationSize))
		gotSubTree := tree.SubTree()
		slices.Sort(gotSubTree)
		if !slices.Equal(gotSubTree, test.wantSubTree) {
			t.Errorf("SubTree() = %v, want %v", gotSubTree, test.wantSubTree)
		}
	}
}

func TestParent(t *testing.T) {
	treeConfigTestData := []struct {
		configurationSize int
		id                hotstuff.ID
		branchFactor      int
		wantParent        hotstuff.ID
	}{
		{10, 1, 2, 1},
		{10, 5, 2, 2},
		{10, 2, 2, 1},
		{10, 3, 2, 1},
		{10, 4, 2, 2},
		{21, 1, 4, 1},
		{21, 1, 3, 1},
		{21, 2, 4, 1},
		{21, 2, 3, 1},
		{21, 9, 3, 3},
		{21, 7, 3, 2},
		{21, 3, 4, 1},
		{21, 10, 4, 3},
		{21, 15, 4, 4},
		{21, 20, 4, 5},
		{21, 5, 4, 1},
	}
	for _, test := range treeConfigTestData {
		tree := CreateTree(test.id, test.branchFactor, replicaIDs(test.configurationSize))

		if gotParent, ok := tree.Parent(); ok {
			if gotParent != test.wantParent {
				t.Errorf("Parent() = %d, want %d", gotParent, test.wantParent)
			}
		}
	}
}

func TestIsRoot(t *testing.T) {
	treeConfigTestData := []struct {
		configurationSize int
		id                hotstuff.ID
		branchFactor      int
		wantIsRoot        bool
	}{
		{10, 1, 2, true},
		{10, 5, 2, false},
		{10, 2, 2, false},
		{10, 3, 2, false},
		{10, 4, 2, false},
		{21, 1, 4, true},
		{21, 1, 3, true},
		{21, 2, 4, false},
		{21, 2, 3, false},
		{21, 9, 3, false},
		{21, 7, 3, false},
		{21, 3, 4, false},
		{21, 10, 4, false},
		{21, 15, 4, false},
		{21, 20, 4, false},
		{21, 5, 4, false},
	}
	for _, test := range treeConfigTestData {
		tree := CreateTree(test.id, test.branchFactor, replicaIDs(test.configurationSize))
		gotIsRoot := tree.IsRoot(test.id)
		if gotIsRoot != test.wantIsRoot {
			t.Errorf("IsRoot() = %t, want %t", gotIsRoot, test.wantIsRoot)
		}
	}
}

func TestReplicaHeight(t *testing.T) {
	treeConfigTestData := []struct {
		configurationSize int
		id                hotstuff.ID
		branchFactor      int
		wantReplicaHeight int
	}{
		{10, 1, 2, 4},
		{10, 5, 2, 2},
		{10, 2, 2, 3},
		{10, 3, 2, 3},
		{10, 4, 2, 2},
		{21, 1, 4, 3},
		{21, 1, 3, 4},
		{21, 2, 4, 2},
		{21, 2, 3, 3},
		{21, 9, 3, 2},
		{21, 7, 3, 2},
		{21, 3, 4, 2},
		{21, 10, 4, 1},
		{21, 15, 4, 1},
		{21, 20, 4, 1},
		{21, 5, 4, 2},
	}
	for _, test := range treeConfigTestData {
		tree := CreateTree(test.id, test.branchFactor, replicaIDs(test.configurationSize))
		gotReplicaHeight := tree.ReplicaHeight()
		if gotReplicaHeight != test.wantReplicaHeight {
			t.Errorf("ReplicaHeight() = %d, want %d", gotReplicaHeight, test.wantReplicaHeight)
		}
	}
}

func TestPeersOf(t *testing.T) {
	treeConfigTestData := []struct {
		configurationSize int
		id                hotstuff.ID
		branchFactor      int
		wantPeers         []hotstuff.ID
	}{
		{10, 1, 2, []hotstuff.ID{}},
		{10, 5, 2, []hotstuff.ID{4, 5}},
		{10, 2, 2, []hotstuff.ID{2, 3}},
		{10, 3, 2, []hotstuff.ID{2, 3}},
		{10, 4, 2, []hotstuff.ID{4, 5}},
		{21, 1, 4, []hotstuff.ID{}},
		{21, 1, 3, []hotstuff.ID{}},
		{21, 2, 4, []hotstuff.ID{2, 3, 4, 5}},
		{21, 2, 3, []hotstuff.ID{2, 3, 4}},
		{21, 9, 3, []hotstuff.ID{8, 9, 10}},
		{21, 7, 3, []hotstuff.ID{5, 6, 7}},
		{21, 3, 4, []hotstuff.ID{2, 3, 4, 5}},
		{21, 10, 4, []hotstuff.ID{10, 11, 12, 13}},
		{21, 15, 4, []hotstuff.ID{14, 15, 16, 17}},
		{21, 20, 4, []hotstuff.ID{18, 19, 20, 21}},
		{21, 5, 4, []hotstuff.ID{2, 3, 4, 5}},
	}
	for _, test := range treeConfigTestData {
		tree := CreateTree(test.id, test.branchFactor, replicaIDs(test.configurationSize))
		gotPeers := tree.PeersOf(test.id)
		slices.Sort(gotPeers)
		if !slices.Equal(gotPeers, test.wantPeers) {
			t.Errorf("PeersOf() = %v, want %v", gotPeers, test.wantPeers)
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
			tree := CreateTree(1, bm.bf, replicaIDs(bm.size))
			// replace `for range b.N` with `for b.Loop()` when go 1.24 released (in release candidate as of writing)
			// for b.Loop() {
			for range b.N {
				tree.ReplicaChildren()
			}
		})
	}
}
