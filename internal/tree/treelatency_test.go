package tree

import (
	"testing"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/latency"
)

func TestTreeLatency(t *testing.T) {
	var (
		sevenLocations   = []string{"Melbourne", "Toronto", "Prague", "Paris", "Tokyo", "Amsterdam", "Auckland"}
		fifteenLocations = []string{
			"Melbourne", "Melbourne", "Toronto", "Toronto", "Prague", "Prague", "Paris", "Paris", "Tokyo",
			"Tokyo", "Amsterdam", "Amsterdam", "Auckland", "Auckland", "Melbourne",
		}
	)
	testData := []struct {
		id    hotstuff.ID
		size  int
		fixed bool
		delta time.Duration
		want  time.Duration
	}{
		{id: 1, size: 7, fixed: false, delta: 0, want: 521775000},
		{id: 2, size: 7, fixed: false, delta: 0, want: 178253000},
		{id: 3, size: 7, fixed: false, delta: 0, want: 279038000},
		{id: 4, size: 7, fixed: false, delta: 0, want: 0},
		{id: 1, size: 15, fixed: false, delta: 0, want: 607507000},
		{id: 2, size: 15, fixed: false, delta: 0, want: 511744000},
		{id: 3, size: 15, fixed: false, delta: 0, want: 388915000},
		{id: 4, size: 15, fixed: false, delta: 0, want: 178253000},
		{id: 5, size: 15, fixed: false, delta: 0, want: 269007000},
		{id: 1, size: 15, fixed: true, delta: 10, want: 60},
		{id: 2, size: 15, fixed: true, delta: 10, want: 40},
		{id: 3, size: 15, fixed: true, delta: 10, want: 40},
		{id: 4, size: 15, fixed: true, delta: 10, want: 20},
		{id: 9, size: 15, fixed: true, delta: 10, want: 0},
		{id: 1, size: 7, fixed: true, delta: 10, want: 40},
		{id: 2, size: 7, fixed: true, delta: 10, want: 20},
		{id: 3, size: 7, fixed: true, delta: 10, want: 20},
		{id: 4, size: 7, fixed: true, delta: 10, want: 0},
	}
	for _, test := range testData {
		var lm latency.Matrix
		if test.size == 7 {
			lm = latency.MatrixFrom(sevenLocations)
		} else {
			lm = latency.MatrixFrom(fifteenLocations)
		}
		var latType LatencyType
		if test.fixed {
			latType = FixedLatency
		} else {
			latType = AggregationLatency
		}

		bf := 2
		treePos := DefaultTreePos(test.size)
		tree := CreateTree(test.id, bf, treePos)
		got := tree.Latency(lm, test.delta, latType)
		if got != test.want {
			t.Errorf("tree.Latency(_, %v, %v) = %v, want %v", test.delta, latType, got, test.want)
		}
	}
}
