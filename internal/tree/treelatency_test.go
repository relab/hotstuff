package tree

import (
	"testing"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/latency"
)

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

func TestAggregationWaitTime(t *testing.T) {
	testDataAggregationTime := []struct {
		id    hotstuff.ID
		locs  []string
		delta time.Duration
		want  time.Duration
	}{
		{id: 1, locs: cities07, delta: 0, want: 521775000},
		{id: 2, locs: cities07, delta: 0, want: 178253000},
		{id: 3, locs: cities07, delta: 0, want: 279038000},
		{id: 4, locs: cities07, delta: 0, want: 0},
		{id: 1, locs: cities15, delta: 0, want: 607507000},
		{id: 2, locs: cities15, delta: 0, want: 511744000},
		{id: 3, locs: cities15, delta: 0, want: 388915000},
		{id: 4, locs: cities15, delta: 0, want: 178253000},
		{id: 5, locs: cities15, delta: 0, want: 269007000},
	}
	for _, test := range testDataAggregationTime {
		bf := 2
		treePos := DefaultTreePos(len(test.locs))
		tree := NewSimple(test.id, bf, treePos)
		lm := latency.MatrixFrom(test.locs)
		tree.SetAggregationWaitTime(lm, test.delta)
		got := tree.WaitTime()
		if got != test.want {
			t.Errorf("tree.WaitTime(%v) = %v, want %v", test.delta, got, test.want)
		}
	}
}

func TestTreeHeightWaitTime(t *testing.T) {
	testDataTreeHeightTime := []struct {
		id    hotstuff.ID
		locs  []string
		delta time.Duration
		want  time.Duration
	}{
		{id: 1, locs: cities15, delta: 10, want: 60},
		{id: 2, locs: cities15, delta: 10, want: 40},
		{id: 3, locs: cities15, delta: 10, want: 40},
		{id: 4, locs: cities15, delta: 10, want: 20},
		{id: 9, locs: cities15, delta: 10, want: 0},
		{id: 1, locs: cities07, delta: 10, want: 40},
		{id: 2, locs: cities07, delta: 10, want: 20},
		{id: 3, locs: cities07, delta: 10, want: 20},
		{id: 4, locs: cities07, delta: 10, want: 0},
	}
	for _, test := range testDataTreeHeightTime {
		bf := 2
		treePos := DefaultTreePos(len(test.locs))
		tree := NewSimple(test.id, bf, treePos)
		tree.SetTreeHeightWaitTime(test.delta)
		got := tree.WaitTime()
		if got != test.want {
			t.Errorf("tree.WaitTime(%v) = %v, want %v", test.delta, got, test.want)
		}
	}
}
