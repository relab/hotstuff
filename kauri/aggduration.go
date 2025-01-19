package kauri

import (
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/latency"
	"github.com/relab/hotstuff/internal/tree"
)

// AggDuration calculates the maximum duration of a path in the tree.
type AggDuration struct {
	tree  *tree.Tree
	lm    latency.Matrix
	id    hotstuff.ID
	delta time.Duration //the time it takes to aggregate the children
}

func NewAggDuration(tree *tree.Tree, id hotstuff.ID, lm latency.Matrix, delta time.Duration) *AggDuration {
	return &AggDuration{tree: tree, lm: lm, id: id, delta: delta}
}

func (t *AggDuration) AggTimerDuration() time.Duration {
	return t.aggTimerDuration(t.id)
}

// aggTimerDuration calculates the network latency at child to aggregate the latency of its children.
// if child is leaf node, so it returns 0.
// any other level, it returns the maximum latency of its children to complete the aggregation.
func (t *AggDuration) aggTimerDuration(id hotstuff.ID) time.Duration {
	children := t.tree.ChildrenOf(id)
	if len(children) == 0 {
		return 0
	}
	latencies := make([]time.Duration, len(children))
	// this logic can be pushed to the recursive function, but in this case one way latency is sufficient.
	for index, child := range children {
		latencies[index] = (2 * t.lm.Latency(id, child)) + t.aggTimerDuration(child)
	}
	return max(latencies) + t.delta
}

func max(latencies []time.Duration) time.Duration {
	if len(latencies) == 0 {
		return 0
	}
	max := latencies[0]
	for _, latency := range latencies {
		if latency > max {
			max = latency
		}
	}
	return max
}
