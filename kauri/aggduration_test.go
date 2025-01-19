package kauri_test

import (
	"testing"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/latency"
	"github.com/relab/hotstuff/internal/tree"
	"github.com/relab/hotstuff/kauri"
)

func TestAggDurationHeight2(t *testing.T) {
	locations := []string{"Melbourne", "Toronto", "Prague", "Paris", "Tokyo", "Amsterdam", "Auckland"}
	lm := latency.MatrixFrom(locations)
	testData := []struct {
		id   hotstuff.ID
		lm   latency.Matrix
		want time.Duration
	}{
		{1, lm, 521775000},
		{2, lm, 178253000},
		{3, lm, 279038000},
		{4, lm, 0},
	}
	treePos := []hotstuff.ID{1, 2, 3, 4, 5, 6, 7}
	for _, test := range testData {
		tree := tree.CreateTree(test.id, 2, treePos)
		agg := kauri.NewAggDuration(tree, test.id, test.lm, 0)
		if agg.AggTimerDuration() != test.want {
			t.Errorf("AggDuration(%d, %v).Duration() = %v; want %v", test.id, test.lm, agg.AggTimerDuration(), test.want)
		}
	}
}

func TestAggDurationHeight3(t *testing.T) {
	locations := []string{"Melbourne", "Melbourne", "Toronto",
		"Toronto", "Prague", "Prague", "Paris", "Paris", "Tokyo",
		"Tokyo", "Amsterdam", "Amsterdam", "Auckland", "Auckland", "Melbourne"}
	lm := latency.MatrixFrom(locations)
	testData := []struct {
		id   hotstuff.ID
		lm   latency.Matrix
		want time.Duration
	}{

		{1, lm, 607507000},
		{2, lm, 511744000},
		{3, lm, 388915000},
		{4, lm, 178253000},
		{5, lm, 269007000},
	}
	treePos := []hotstuff.ID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	for _, test := range testData {
		tree := tree.CreateTree(test.id, 2, treePos)
		agg := kauri.NewAggDuration(tree, test.id, test.lm, 0)
		if agg.AggTimerDuration() != test.want {
			t.Errorf("AggDuration(%d, %v).Duration() = %v; want %v", test.id, test.lm, agg.AggTimerDuration(), test.want)
		}
	}
}
