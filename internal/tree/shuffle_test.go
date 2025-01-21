package tree_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/relab/hotstuff/internal/tree"
)

func TestTreeShuffle(t *testing.T) {
	tests := []struct {
		size int
	}{
		{size: 0},
		{size: 1},
		{size: 2},
		{size: 3},
		{size: 4},
		{size: 5},
		{size: 10},
		{size: 20},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("size=%d", tt.size), func(t *testing.T) {
			treePos := tree.DefaultTreePosUint32(tt.size)
			tree.Shuffle(treePos)
			if len(treePos) != tt.size {
				t.Errorf("Randomize() got %v, want %v", len(treePos), tt.size)
			}
			want := tree.DefaultTreePosUint32(tt.size)
			for _, w := range want {
				if !slices.Contains(treePos, w) {
					t.Errorf("Randomize() = %v, want elements %v, missing %v", treePos, want, w)
				}
			}
		})
	}
}

func BenchmarkTreeShuffle(b *testing.B) {
	for _, size := range []int{10, 100, 1000, 10000} {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			treePos := tree.DefaultTreePosUint32(size)
			for range b.N {
				tree.Shuffle(treePos)
			}
		})
	}
}
