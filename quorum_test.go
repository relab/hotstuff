package hotstuff

import (
	"fmt"
	"testing"
)

func TestQuorumSize(t *testing.T) {
	tests := []struct {
		n    int
		want int
	}{
		{n: 4, want: 3},   // f=1
		{n: 5, want: 4},   // f=1
		{n: 6, want: 4},   // f=1
		{n: 7, want: 5},   // f=2
		{n: 8, want: 6},   // f=2
		{n: 9, want: 6},   // f=2
		{n: 10, want: 7},  // f=3
		{n: 11, want: 8},  // f=3
		{n: 12, want: 8},  // f=3
		{n: 13, want: 9},  // f=4
		{n: 14, want: 10}, // f=4
		{n: 15, want: 10}, // f=4
		{n: 16, want: 11}, // f=5
		{n: 17, want: 12}, // f=5
		{n: 18, want: 12}, // f=5
		{n: 19, want: 13}, // f=6
		{n: 20, want: 14}, // f=6
		{n: 21, want: 14}, // f=6
		{n: 22, want: 15}, // f=7
		{n: 23, want: 16}, // f=7
		{n: 24, want: 16}, // f=7
		{n: 25, want: 17}, // f=8
		{n: 26, want: 18}, // f=8
		{n: 27, want: 18}, // f=8
		{n: 31, want: 21}, // f=10
		{n: 32, want: 22}, // f=10
		{n: 33, want: 22}, // f=10
		{n: 34, want: 23}, // f=11
		{n: 35, want: 24}, // f=11
		{n: 36, want: 24}, // f=11
		{n: 37, want: 25}, // f=12
		{n: 38, want: 26}, // f=12
		{n: 39, want: 26}, // f=12
		{n: 40, want: 27}, // f=13
		{n: 41, want: 28}, // f=13
		{n: 42, want: 28}, // f=13
		{n: 43, want: 29}, // f=14
		{n: 44, want: 30}, // f=14
		{n: 45, want: 30}, // f=14
		{n: 73, want: 49}, // f=24
		{n: 74, want: 50}, // f=24
		{n: 75, want: 50}, // f=24
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("n=%d", tt.n), func(t *testing.T) {
			if got := QuorumSize(tt.n); got != tt.want {
				t.Errorf("QuorumSize(%d) = %d; want %d", tt.n, got, tt.want)
			}
		})
	}
}
