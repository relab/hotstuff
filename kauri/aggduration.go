package kauri

import (
	"slices"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/latency"
	"github.com/relab/hotstuff/internal/tree"
	"github.com/relab/hotstuff/modules"
)

// AggregationLatency calculates the maximum duration of a path in the tree.
type AggregationLatency struct {
	tree      *tree.Tree
	lm        latency.Matrix
	id        hotstuff.ID
	delta     time.Duration // the time it takes to aggregate the children
	timerType modules.WaitTimerType
}

func NewAggregationLatency(tree *tree.Tree, lm latency.Matrix, opts *modules.Options) *AggregationLatency {
	return &AggregationLatency{
		tree:      tree,
		lm:        lm,
		id:        opts.ID(),
		delta:     opts.TreeConfig().TreeWaitDelta(),
		timerType: opts.TreeConfig().WaitTimerType(),
	}
}

func (t *AggregationLatency) WaitTimerDuration() time.Duration {
	if t.timerType == modules.WaitTimerAgg {
		return t.aggregationLatency(t.id)
	}
	return t.fixedAggDuration()
}

// aggregationLatency returns the highest latency path from node id to its leaf nodes.
//
// If the node is a leaf, it returns 0 as no aggregation is required.
// For other nodes, the aggregation latency for a child includes:
// - Round-trip latency to the child
// - Aggregation latency required by the child node (recursive call)
func (t *AggregationLatency) aggregationLatency(id hotstuff.ID) time.Duration {
	children := t.tree.ChildrenOf(id)
	if len(children) == 0 {
		return 0 // base case: leaf nodes have zero aggregation latency.
	}
	// calculate aggregation latencies for each child
	latencies := make([]time.Duration, len(children))
	for i, child := range children {
		latencies[i] = 2*t.lm.Latency(id, child) + t.aggregationLatency(child)
	}
	return max(latencies) + t.delta
}

// FixedAggDuration returns the fixed aggregation duration based on the height of the tree.
func (t *AggregationLatency) fixedAggDuration() time.Duration {
	return time.Duration(2*(t.tree.ReplicaHeight()-1)) * t.delta
}

func max(latencies []time.Duration) time.Duration {
	if len(latencies) == 0 {
		return 0
	}
	return slices.Max(latencies)
}
