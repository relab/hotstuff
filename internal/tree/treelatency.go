package tree

import (
	"slices"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/latency"
)

// WaitTime returns the expected time to wait for the aggregation of votes.
func (t *Tree) WaitTime() time.Duration {
	return t.waitTime
}

// SetAggregationWaitTime sets the wait time for the aggregation of votes based on the
// highest latency path from node id to its leaf nodes.
// Only one of SetAggregationWaitTime or SetTreeHeightWaitTime should be called.
func (t *Tree) SetAggregationWaitTime(lm latency.Matrix, delta time.Duration) {
	t.waitTime = t.aggregationTime(t.id, lm, delta)
}

// SetTreeHeightWaitTime sets the wait time for the aggregation of votes based on the
// height of the tree.
// Only one of SetAggregationWaitTime or SetTreeHeightWaitTime should be called.
func (t *Tree) SetTreeHeightWaitTime(delta time.Duration) {
	t.waitTime = t.treeHeightTime(delta)
}

// aggregationTime returns the time to wait for the aggregation of votes based on the
// highest latency path from node id to its leaf nodes.
// The id is required because the function is recursive.
//
// If the node is a leaf, it returns 0 as no aggregation is required.
// For other nodes, the aggregation time for a child includes:
// - Round-trip time to the child
// - Aggregation time required by the child node (recursive call)
func (t *Tree) aggregationTime(id hotstuff.ID, lm latency.Matrix, delta time.Duration) time.Duration {
	children := t.ChildrenOf(id)
	if len(children) == 0 {
		return 0 // base case: leaf nodes have zero aggregation latency.
	}
	// calculate aggregation latencies for each child
	latencies := make([]time.Duration, len(children))
	for i, child := range children {
		latencies[i] = 2*lm.Latency(id, child) + t.aggregationTime(child, lm, delta)
	}
	return max(latencies) + delta
}

// treeHeightTime returns a fixed time to wait based on the height of the tree.
func (t *Tree) treeHeightTime(delta time.Duration) time.Duration {
	return time.Duration(2*(t.ReplicaHeight()-1)) * delta
}

func max(latencies []time.Duration) time.Duration {
	if len(latencies) == 0 {
		return 0
	}
	return slices.Max(latencies)
}
