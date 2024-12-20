package latency

import (
	"slices"
	"time"

	"github.com/relab/hotstuff"
)

// Between returns the latency between locations a and b.
// If a or b are not valid locations, the function will panic.
func Between(a, b string) time.Duration {
	fromIdx, toIdx := slices.Index(allLocations, a), slices.Index(allLocations, b)
	return allLatencies[fromIdx][toIdx]
}

// Matrix represents a latency matrix and locations.
type Matrix struct {
	enabled bool
	lm      [][]time.Duration
	locs    []string
}

// MatrixFrom returns the latencies between the given locations.
func MatrixFrom(locations []string) Matrix {
	locationIndices := make([]int, len(locations))
	for i, loc := range locations {
		locationIndices[i] = slices.Index(allLocations, loc)
	}
	newLatencies := make([][]time.Duration, len(locationIndices))
	for i, fromIdx := range locationIndices {
		newLatencies[i] = make([]time.Duration, len(locationIndices))
		for j, toIdx := range locationIndices {
			newLatencies[i][j] = allLatencies[fromIdx][toIdx]
		}
	}
	return Matrix{
		enabled: true,
		lm:      newLatencies,
		locs:    locations,
	}
}

// Latency returns the latency between nodes a and b.
// If a or b are not valid nodes, the function will panic.
func (lm Matrix) Latency(a, b hotstuff.ID) time.Duration {
	return lm.lm[a-1][b-1]
}

// Location returns the location of the node with the given ID.
func (lm Matrix) Location(id hotstuff.ID) string {
	return lm.locs[id-1]
}

// Enabled returns true if a LatencyMatrix was provided.
func (lm Matrix) Enabled() bool {
	return lm.enabled
}
