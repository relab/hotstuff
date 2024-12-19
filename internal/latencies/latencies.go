package latencies

import (
	"slices"
	"time"

	"github.com/relab/hotstuff"
)

// Latency returns the latency between locations a and b.
// If a or b are not valid locations, the function will panic.
func Latency(a, b string) time.Duration {
	fromIdx, toIdx := slices.Index(locations, a), slices.Index(locations, b)
	return latencies[fromIdx][toIdx]
}

// LatencyMatrix created by LatenciesFrom.
type LatencyMatrix [][]time.Duration

// LatenciesFrom returns the latencies between the given locations.
func LatenciesFrom(locs []string) LatencyMatrix {
	locationIndices := make([]int, len(locs))
	for i, loc := range locs {
		locationIndices[i] = slices.Index(locations, loc)
	}
	newLatencies := make(LatencyMatrix, len(locationIndices))
	for i, fromIdx := range locationIndices {
		newLatencies[i] = make([]time.Duration, len(locs))
		for j, toIdx := range locationIndices {
			newLatencies[i][j] = latencies[fromIdx][toIdx]
		}
	}
	return newLatencies
}

// Latency returns the latency between nodes a and b.
// If a or b are not valid nodes, the function will panic.
func (lm LatencyMatrix) Latency(a, b hotstuff.ID) time.Duration {
	return lm[a-1][b-1]
}
