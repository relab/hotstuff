package latencies

import (
	"slices"
	"time"

	"github.com/relab/hotstuff"
)

// LatencyCity returns the latency between from and to locations.
func LatencyCity(from, to string) time.Duration {
	fromIdx, toIdx := slices.Index(locations, from), slices.Index(locations, to)
	return latencies[fromIdx][toIdx]
}

// LocationName returns the location name at the given index.
func LocationName(index int) string {
	return locations[index]
}

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

// LatencyID returns the latency between nodes a and b.
func (lm LatencyMatrix) LatencyID(a, b hotstuff.ID) time.Duration {
	return lm[a-1][b-1]
}
