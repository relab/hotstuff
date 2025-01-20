package kauri_test

import (
	"testing"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/latency"
	"github.com/relab/hotstuff/internal/tree"
	"github.com/relab/hotstuff/kauri"
	"github.com/relab/hotstuff/modules"
)

func makeAgg(
	id hotstuff.ID,
	delta int,
	bf int,
	timerType modules.WaitTimerType,
	treePos []hotstuff.ID,
	lm latency.Matrix,
) *kauri.AggregationLatency {
	opts := modules.OptionsWithID(id)
	opts.SetTreeConfig(uint32(bf), treePos, time.Duration(delta), timerType)
	tree := tree.CreateTree(id, bf, treePos)
	return kauri.NewAggregationLatency(tree, lm, opts)
}

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
	bf := 2
	delta := 0
	for _, test := range testData {
		agg := makeAgg(test.id, delta, bf, modules.WaitTimerAgg, treePos, test.lm)
		if agg.WaitTimerDuration() != test.want {
			t.Errorf("AggDuration(%d, %v).Duration() = %v; want %v", test.id, test.lm, agg.WaitTimerDuration(), test.want)
		}
	}
}

func TestAggDurationHeight3(t *testing.T) {
	locations := []string{
		"Melbourne", "Melbourne", "Toronto",
		"Toronto", "Prague", "Prague", "Paris", "Paris", "Tokyo",
		"Tokyo", "Amsterdam", "Amsterdam", "Auckland", "Auckland", "Melbourne",
	}
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
	bf := 2
	delta := 0
	for _, test := range testData {
		agg := makeAgg(test.id, delta, bf, modules.WaitTimerAgg, treePos, test.lm)
		if agg.WaitTimerDuration() != test.want {
			t.Errorf("AggDuration(%d, %v).Duration() = %v; want %v", test.id, test.lm, agg.WaitTimerDuration(), test.want)
		}
	}
}

func TestFixedAggDurationH4(t *testing.T) {
	treePos := []hotstuff.ID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	testData := []struct {
		id   hotstuff.ID
		want time.Duration
	}{
		{1, 60},
		{2, 40},
		{3, 40},
		{4, 20},
		{9, 0},
	}
	delta := 10
	bf := 2
	for _, test := range testData {
		agg := makeAgg(test.id, delta, bf, modules.WaitTimerFixed, treePos, latency.Matrix{})
		if agg.WaitTimerDuration() != test.want {
			t.Errorf("FixedAggDuration(%d).Duration() = %v; want %v", test.id, agg.WaitTimerDuration(), test.want)
		}
	}
}

func TestFixedAggDuration(t *testing.T) {
	treePos := []hotstuff.ID{1, 2, 3, 4, 5, 6, 7}
	testData := []struct {
		id   hotstuff.ID
		want time.Duration
	}{
		{1, 40},
		{2, 20},
		{3, 20},
		{4, 0},
	}
	delta := 10
	bf := 2
	for _, test := range testData {
		agg := makeAgg(test.id, delta, bf, modules.WaitTimerFixed, treePos, latency.Matrix{})
		if agg.WaitTimerDuration() != test.want {
			t.Errorf("FixedAggDuration(%d).Duration() = %v; want %v", test.id, agg.WaitTimerDuration(), test.want)
		}
	}
}
