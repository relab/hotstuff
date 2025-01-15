package tree

import (
	"fmt"
	"slices"
	"testing"

	"github.com/relab/hotstuff"
)

func TestCreateTree(t *testing.T) {
	tests := []struct {
		size         int
		id           hotstuff.ID
		branchFactor int
		wantHeight   int
	}{
		{size: 10, id: 1, branchFactor: 2, wantHeight: 4},
		{size: 21, id: 1, branchFactor: 4, wantHeight: 3},
		{size: 21, id: 1, branchFactor: 3, wantHeight: 4},
		{size: 43, id: 1, branchFactor: 5, wantHeight: 4},
		{size: 43, id: 1, branchFactor: 4, wantHeight: 4},
		{size: 111, id: 1, branchFactor: 10, wantHeight: 3},
		{size: 111, id: 1, branchFactor: 3, wantHeight: 5},
	}
	for _, test := range tests {
		tree := CreateTree(test.id, test.branchFactor, DefaultTreePos(test.size))
		if tree.TreeHeight() != test.wantHeight {
			t.Errorf("CreateTree(%d, %d, %d).GetTreeHeight() = %d, want %d",
				test.size, test.id, test.branchFactor, tree.TreeHeight(), test.wantHeight)
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
		size           int
		id             hotstuff.ID
		branchFactor   int
		wantTreeHeight int
	}{
		{size: 10, id: 1, branchFactor: 2, wantTreeHeight: 4},
		{size: 10, id: 5, branchFactor: 2, wantTreeHeight: 4},
		{size: 10, id: 2, branchFactor: 2, wantTreeHeight: 4},
		{size: 10, id: 3, branchFactor: 2, wantTreeHeight: 4},
		{size: 10, id: 4, branchFactor: 2, wantTreeHeight: 4},
		{size: 21, id: 1, branchFactor: 4, wantTreeHeight: 3},
		{size: 21, id: 1, branchFactor: 3, wantTreeHeight: 4},
		{size: 21, id: 2, branchFactor: 4, wantTreeHeight: 3},
		{size: 21, id: 2, branchFactor: 3, wantTreeHeight: 4},
		{size: 21, id: 9, branchFactor: 3, wantTreeHeight: 4},
		{size: 21, id: 10, branchFactor: 4, wantTreeHeight: 3},
	}
	for _, test := range treeTestData {
		tree := CreateTree(test.id, test.branchFactor, DefaultTreePos(test.size))
		gotTreeHeight := tree.TreeHeight()
		if gotTreeHeight != test.wantTreeHeight {
			t.Errorf("TreeHeight() = %d, want %d", gotTreeHeight, test.wantTreeHeight)
		}
	}
}

func TestReplicaChildren(t *testing.T) {
	treeConfigTestData := []struct {
		size         int
		id           hotstuff.ID
		branchFactor int
		wantChildren []hotstuff.ID
	}{
		{size: 10, id: 1, branchFactor: 2, wantChildren: []hotstuff.ID{2, 3}},
		{size: 10, id: 5, branchFactor: 2, wantChildren: []hotstuff.ID{10}},
		{size: 10, id: 2, branchFactor: 2, wantChildren: []hotstuff.ID{4, 5}},
		{size: 10, id: 3, branchFactor: 2, wantChildren: []hotstuff.ID{6, 7}},
		{size: 10, id: 4, branchFactor: 2, wantChildren: []hotstuff.ID{8, 9}},
		{size: 21, id: 1, branchFactor: 4, wantChildren: []hotstuff.ID{2, 3, 4, 5}},
		{size: 21, id: 1, branchFactor: 3, wantChildren: []hotstuff.ID{2, 3, 4}},
		{size: 21, id: 2, branchFactor: 4, wantChildren: []hotstuff.ID{6, 7, 8, 9}},
		{size: 21, id: 2, branchFactor: 3, wantChildren: []hotstuff.ID{5, 6, 7}},
		{size: 21, id: 9, branchFactor: 3, wantChildren: []hotstuff.ID{}},
		{size: 21, id: 7, branchFactor: 3, wantChildren: []hotstuff.ID{20, 21}},
		{size: 21, id: 3, branchFactor: 4, wantChildren: []hotstuff.ID{10, 11, 12, 13}},
		{size: 21, id: 10, branchFactor: 4, wantChildren: []hotstuff.ID{}},
		{size: 21, id: 15, branchFactor: 4, wantChildren: []hotstuff.ID{}},
		{size: 21, id: 20, branchFactor: 4, wantChildren: []hotstuff.ID{}},
		{size: 21, id: 5, branchFactor: 4, wantChildren: []hotstuff.ID{18, 19, 20, 21}},
	}
	for _, test := range treeConfigTestData {
		tree := CreateTree(test.id, test.branchFactor, DefaultTreePos(test.size))
		gotChildren := tree.ReplicaChildren()
		slices.Sort(gotChildren)
		if !slices.Equal(gotChildren, test.wantChildren) {
			t.Errorf("ReplicaChildren() = %v, want %v", gotChildren, test.wantChildren)
		}
	}
}

func TestSubTree(t *testing.T) {
	treeConfigTestData := []struct {
		size         int
		id           hotstuff.ID
		branchFactor int
		wantSubTree  []hotstuff.ID
	}{
		{size: 10, id: 1, branchFactor: 2, wantSubTree: []hotstuff.ID{2, 3, 4, 5, 6, 7, 8, 9, 10}},
		{size: 10, id: 5, branchFactor: 2, wantSubTree: []hotstuff.ID{10}},
		{size: 10, id: 2, branchFactor: 2, wantSubTree: []hotstuff.ID{4, 5, 8, 9, 10}},
		{size: 10, id: 3, branchFactor: 2, wantSubTree: []hotstuff.ID{6, 7}},
		{size: 10, id: 4, branchFactor: 2, wantSubTree: []hotstuff.ID{8, 9}},
		{size: 21, id: 1, branchFactor: 4, wantSubTree: []hotstuff.ID{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21}},
		{size: 21, id: 1, branchFactor: 3, wantSubTree: []hotstuff.ID{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21}},
		{size: 21, id: 2, branchFactor: 4, wantSubTree: []hotstuff.ID{6, 7, 8, 9}},
		{size: 21, id: 2, branchFactor: 3, wantSubTree: []hotstuff.ID{5, 6, 7, 14, 15, 16, 17, 18, 19, 20, 21}},
		{size: 21, id: 9, branchFactor: 3, wantSubTree: []hotstuff.ID{}},
		{size: 21, id: 7, branchFactor: 3, wantSubTree: []hotstuff.ID{20, 21}},
		{size: 21, id: 3, branchFactor: 4, wantSubTree: []hotstuff.ID{10, 11, 12, 13}},
		{size: 21, id: 10, branchFactor: 4, wantSubTree: []hotstuff.ID{}},
		{size: 21, id: 15, branchFactor: 4, wantSubTree: []hotstuff.ID{}},
		{size: 21, id: 20, branchFactor: 4, wantSubTree: []hotstuff.ID{}},
		{size: 21, id: 5, branchFactor: 4, wantSubTree: []hotstuff.ID{18, 19, 20, 21}},
	}
	for _, test := range treeConfigTestData {
		tree := CreateTree(test.id, test.branchFactor, DefaultTreePos(test.size))
		gotSubTree := tree.SubTree()
		slices.Sort(gotSubTree)
		if !slices.Equal(gotSubTree, test.wantSubTree) {
			t.Errorf("SubTree() = %v, want %v", gotSubTree, test.wantSubTree)
		}
	}
}

func TestParent(t *testing.T) {
	treeConfigTestData := []struct {
		size         int
		id           hotstuff.ID
		branchFactor int
		wantParent   hotstuff.ID
	}{
		{size: 10, id: 1, branchFactor: 2, wantParent: 1},
		{size: 10, id: 5, branchFactor: 2, wantParent: 2},
		{size: 10, id: 2, branchFactor: 2, wantParent: 1},
		{size: 10, id: 3, branchFactor: 2, wantParent: 1},
		{size: 10, id: 4, branchFactor: 2, wantParent: 2},
		{size: 21, id: 1, branchFactor: 4, wantParent: 1},
		{size: 21, id: 1, branchFactor: 3, wantParent: 1},
		{size: 21, id: 2, branchFactor: 4, wantParent: 1},
		{size: 21, id: 2, branchFactor: 3, wantParent: 1},
		{size: 21, id: 9, branchFactor: 3, wantParent: 3},
		{size: 21, id: 7, branchFactor: 3, wantParent: 2},
		{size: 21, id: 3, branchFactor: 4, wantParent: 1},
		{size: 21, id: 10, branchFactor: 4, wantParent: 3},
		{size: 21, id: 15, branchFactor: 4, wantParent: 4},
		{size: 21, id: 20, branchFactor: 4, wantParent: 5},
		{size: 21, id: 5, branchFactor: 4, wantParent: 1},
	}
	for _, test := range treeConfigTestData {
		tree := CreateTree(test.id, test.branchFactor, DefaultTreePos(test.size))

		if gotParent, ok := tree.Parent(); ok {
			if gotParent != test.wantParent {
				t.Errorf("Parent() = %d, want %d", gotParent, test.wantParent)
			}
		}
	}
}

func TestIsRoot(t *testing.T) {
	treeConfigTestData := []struct {
		size         int
		id           hotstuff.ID
		branchFactor int
		wantIsRoot   bool
	}{
		{size: 10, id: 1, branchFactor: 2, wantIsRoot: true},
		{size: 10, id: 5, branchFactor: 2, wantIsRoot: false},
		{size: 10, id: 2, branchFactor: 2, wantIsRoot: false},
		{size: 10, id: 3, branchFactor: 2, wantIsRoot: false},
		{size: 10, id: 4, branchFactor: 2, wantIsRoot: false},
		{size: 21, id: 1, branchFactor: 4, wantIsRoot: true},
		{size: 21, id: 1, branchFactor: 3, wantIsRoot: true},
		{size: 21, id: 2, branchFactor: 4, wantIsRoot: false},
		{size: 21, id: 2, branchFactor: 3, wantIsRoot: false},
		{size: 21, id: 9, branchFactor: 3, wantIsRoot: false},
		{size: 21, id: 7, branchFactor: 3, wantIsRoot: false},
		{size: 21, id: 3, branchFactor: 4, wantIsRoot: false},
		{size: 21, id: 10, branchFactor: 4, wantIsRoot: false},
		{size: 21, id: 15, branchFactor: 4, wantIsRoot: false},
		{size: 21, id: 20, branchFactor: 4, wantIsRoot: false},
		{size: 21, id: 5, branchFactor: 4, wantIsRoot: false},
	}
	for _, test := range treeConfigTestData {
		tree := CreateTree(test.id, test.branchFactor, DefaultTreePos(test.size))
		gotIsRoot := tree.IsRoot(test.id)
		if gotIsRoot != test.wantIsRoot {
			t.Errorf("IsRoot() = %t, want %t", gotIsRoot, test.wantIsRoot)
		}
	}
}

func TestReplicaHeight(t *testing.T) {
	treeConfigTestData := []struct {
		size              int
		id                hotstuff.ID
		branchFactor      int
		wantReplicaHeight int
	}{
		{size: 10, id: 1, branchFactor: 2, wantReplicaHeight: 4},
		{size: 10, id: 5, branchFactor: 2, wantReplicaHeight: 2},
		{size: 10, id: 2, branchFactor: 2, wantReplicaHeight: 3},
		{size: 10, id: 3, branchFactor: 2, wantReplicaHeight: 3},
		{size: 10, id: 4, branchFactor: 2, wantReplicaHeight: 2},
		{size: 21, id: 1, branchFactor: 4, wantReplicaHeight: 3},
		{size: 21, id: 1, branchFactor: 3, wantReplicaHeight: 4},
		{size: 21, id: 2, branchFactor: 4, wantReplicaHeight: 2},
		{size: 21, id: 2, branchFactor: 3, wantReplicaHeight: 3},
		{size: 21, id: 9, branchFactor: 3, wantReplicaHeight: 2},
		{size: 21, id: 7, branchFactor: 3, wantReplicaHeight: 2},
		{size: 21, id: 3, branchFactor: 4, wantReplicaHeight: 2},
		{size: 21, id: 10, branchFactor: 4, wantReplicaHeight: 1},
		{size: 21, id: 15, branchFactor: 4, wantReplicaHeight: 1},
		{size: 21, id: 20, branchFactor: 4, wantReplicaHeight: 1},
		{size: 21, id: 5, branchFactor: 4, wantReplicaHeight: 2},
	}
	for _, test := range treeConfigTestData {
		tree := CreateTree(test.id, test.branchFactor, DefaultTreePos(test.size))
		gotReplicaHeight := tree.ReplicaHeight()
		if gotReplicaHeight != test.wantReplicaHeight {
			t.Errorf("ReplicaHeight() = %d, want %d", gotReplicaHeight, test.wantReplicaHeight)
		}
	}
}

func TestPeersOf(t *testing.T) {
	treeConfigTestData := []struct {
		size         int
		id           hotstuff.ID
		branchFactor int
		wantPeers    []hotstuff.ID
	}{
		{size: 10, id: 1, branchFactor: 2, wantPeers: []hotstuff.ID{}},
		{size: 10, id: 5, branchFactor: 2, wantPeers: []hotstuff.ID{4, 5}},
		{size: 10, id: 2, branchFactor: 2, wantPeers: []hotstuff.ID{2, 3}},
		{size: 10, id: 3, branchFactor: 2, wantPeers: []hotstuff.ID{2, 3}},
		{size: 10, id: 4, branchFactor: 2, wantPeers: []hotstuff.ID{4, 5}},
		{size: 21, id: 1, branchFactor: 4, wantPeers: []hotstuff.ID{}},
		{size: 21, id: 1, branchFactor: 3, wantPeers: []hotstuff.ID{}},
		{size: 21, id: 2, branchFactor: 4, wantPeers: []hotstuff.ID{2, 3, 4, 5}},
		{size: 21, id: 2, branchFactor: 3, wantPeers: []hotstuff.ID{2, 3, 4}},
		{size: 21, id: 9, branchFactor: 3, wantPeers: []hotstuff.ID{8, 9, 10}},
		{size: 21, id: 7, branchFactor: 3, wantPeers: []hotstuff.ID{5, 6, 7}},
		{size: 21, id: 3, branchFactor: 4, wantPeers: []hotstuff.ID{2, 3, 4, 5}},
		{size: 21, id: 10, branchFactor: 4, wantPeers: []hotstuff.ID{10, 11, 12, 13}},
		{size: 21, id: 15, branchFactor: 4, wantPeers: []hotstuff.ID{14, 15, 16, 17}},
		{size: 21, id: 20, branchFactor: 4, wantPeers: []hotstuff.ID{18, 19, 20, 21}},
		{size: 21, id: 5, branchFactor: 4, wantPeers: []hotstuff.ID{2, 3, 4, 5}},
	}
	for _, test := range treeConfigTestData {
		tree := CreateTree(test.id, test.branchFactor, DefaultTreePos(test.size))
		gotPeers := tree.PeersOf()
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
			tree := CreateTree(1, bm.bf, DefaultTreePos(bm.size))
			// replace `for range b.N` with `for b.Loop()` when go 1.24 released (in release candidate as of writing)
			// for b.Loop() {
			for range b.N {
				tree.ReplicaChildren()
			}
		})
	}
}
