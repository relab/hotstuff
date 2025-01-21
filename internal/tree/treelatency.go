package tree

import (
	"slices"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/latency"
	"github.com/relab/hotstuff/modules"
)

func (t *Tree) WaitTimerDuration(lm latency.Matrix, opts *modules.Options) time.Duration {
	if opts.TreeConfig().WaitTimerType() == modules.WaitTimerAgg {
		return t.aggregationLatency(t.id, lm, opts.TreeConfig().TreeWaitDelta())
	}
	return t.fixedAggDuration(opts.TreeConfig().TreeWaitDelta())
}

// aggregationLatency returns the highest latency path from node id to its leaf nodes.
//
// If the node is a leaf, it returns 0 as no aggregation is required.
// For other nodes, the aggregation latency for a child includes:
// - Round-trip latency to the child
// - Aggregation latency required by the child node (recursive call)
// id is required due to recursive call.
func (t *Tree) aggregationLatency(id hotstuff.ID, lm latency.Matrix, delta time.Duration) time.Duration {
	children := t.ChildrenOf(id)
	if len(children) == 0 {
		return 0 // base case: leaf nodes have zero aggregation latency.
	}
	// calculate aggregation latencies for each child
	latencies := make([]time.Duration, len(children))
	for i, child := range children {
		latencies[i] = 2*lm.Latency(id, child) + t.aggregationLatency(child, lm, delta)
	}
	return max(latencies) + delta
}

// FixedAggDuration returns the fixed aggregation duration based on the height of the tree.
func (t *Tree) fixedAggDuration(delta time.Duration) time.Duration {
	return time.Duration(2*(t.ReplicaHeight()-1)) * delta
}

func max(latencies []time.Duration) time.Duration {
	if len(latencies) == 0 {
		return 0
	}
	return slices.Max(latencies)
}
