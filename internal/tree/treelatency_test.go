package tree

import (
	"testing"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/latency"
)

func TestWaitTime(t *testing.T) {
	var (
		cities07 = []string{
			"Melbourne", "Toronto", "Prague", "Paris",
			"Tokyo", "Amsterdam", "Auckland",
		}
		cities15 = []string{
			"Melbourne", "Melbourne", "Toronto", "Toronto", "Prague",
			"Prague", "Paris", "Paris", "Tokyo", "Tokyo", "Amsterdam",
			"Amsterdam", "Auckland", "Auckland", "Melbourne",
		}
	)
	testData := []struct {
		id      hotstuff.ID
		locs    []string
		latType DelayType
		delta   time.Duration
		want    time.Duration
	}{
		{id: 1, locs: cities07, latType: AggregationTime, delta: 0, want: 521775000},
		{id: 2, locs: cities07, latType: AggregationTime, delta: 0, want: 178253000},
		{id: 3, locs: cities07, latType: AggregationTime, delta: 0, want: 279038000},
		{id: 4, locs: cities07, latType: AggregationTime, delta: 0, want: 0},
		{id: 1, locs: cities15, latType: AggregationTime, delta: 0, want: 607507000},
		{id: 2, locs: cities15, latType: AggregationTime, delta: 0, want: 511744000},
		{id: 3, locs: cities15, latType: AggregationTime, delta: 0, want: 388915000},
		{id: 4, locs: cities15, latType: AggregationTime, delta: 0, want: 178253000},
		{id: 5, locs: cities15, latType: AggregationTime, delta: 0, want: 269007000},
		{id: 1, locs: cities15, latType: TreeHeightTime, delta: 10, want: 60},
		{id: 2, locs: cities15, latType: TreeHeightTime, delta: 10, want: 40},
		{id: 3, locs: cities15, latType: TreeHeightTime, delta: 10, want: 40},
		{id: 4, locs: cities15, latType: TreeHeightTime, delta: 10, want: 20},
		{id: 9, locs: cities15, latType: TreeHeightTime, delta: 10, want: 0},
		{id: 1, locs: cities07, latType: TreeHeightTime, delta: 10, want: 40},
		{id: 2, locs: cities07, latType: TreeHeightTime, delta: 10, want: 20},
		{id: 3, locs: cities07, latType: TreeHeightTime, delta: 10, want: 20},
		{id: 4, locs: cities07, latType: TreeHeightTime, delta: 10, want: 0},
	}
	for _, test := range testData {
		bf := 2
		treePos := DefaultTreePos(len(test.locs))
		tree := CreateTree(test.id, bf, treePos)
		lm := latency.MatrixFrom(test.locs)
		got := tree.WaitTime(lm, test.delta, test.latType)
		if got != test.want {
			t.Errorf("tree.Latency(_, %v, %v) = %v, want %v", test.delta, test.latType, got, test.want)
		}
	}
}
