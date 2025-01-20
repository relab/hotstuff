package tree

import (
	"testing"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/latency"
	"github.com/relab/hotstuff/modules"
)

func aggTimer(
	id hotstuff.ID,
	delta int,
	bf int,
	timerType modules.WaitTimerType,
	treePos []hotstuff.ID,
	lm latency.Matrix,
) time.Duration {
	opts := modules.OptionsWithID(id)
	opts.SetTreeConfig(uint32(bf), treePos, time.Duration(delta), timerType)
	tree := CreateTree(id, bf, treePos)
	return tree.WaitTimerDuration(lm, opts)
}

var sevenLocations = []string{"Melbourne", "Toronto", "Prague", "Paris", "Tokyo", "Amsterdam", "Auckland"}
var fifteenLocations = []string{"Melbourne", "Melbourne", "Toronto", "Toronto", "Prague", "Prague", "Paris", "Paris", "Tokyo",
	"Tokyo", "Amsterdam", "Amsterdam", "Auckland", "Auckland", "Melbourne"}

func TestAggDurationHeight(t *testing.T) {
	testData := []struct {
		id    hotstuff.ID
		size  int
		fixed bool
		delta int
		want  time.Duration
	}{
		{1, 7, false, 0, 521775000},
		{2, 7, false, 0, 178253000},
		{3, 7, false, 0, 279038000},
		{4, 7, false, 0, 0},
		{1, 15, false, 0, 607507000},
		{2, 15, false, 0, 511744000},
		{3, 15, false, 0, 388915000},
		{4, 15, false, 0, 178253000},
		{5, 15, false, 0, 269007000},
		{1, 15, true, 10, 60},
		{2, 15, true, 10, 40},
		{3, 15, true, 10, 40},
		{4, 15, true, 10, 20},
		{9, 15, true, 10, 0},
		{1, 7, true, 10, 40},
		{2, 7, true, 10, 20},
		{3, 7, true, 10, 20},
		{4, 7, true, 10, 0},
	}
	for _, test := range testData {
		bf := 2
		treePos := DefaultTreePos(test.size)
		var lm latency.Matrix
		if test.size == 7 {
			lm = latency.MatrixFrom(sevenLocations)
		} else {
			lm = latency.MatrixFrom(fifteenLocations)
		}
		var timerType modules.WaitTimerType
		if test.fixed {
			timerType = modules.WaitTimerFixed
		} else {
			timerType = modules.WaitTimerAgg
		}
		agg := aggTimer(test.id, test.delta, bf, timerType, treePos, lm)
		if agg != test.want {
			t.Errorf("AggDuration(%d, %v).Duration() = %v; want %v", test.id, lm, agg, test.want)
		}
	}
}
