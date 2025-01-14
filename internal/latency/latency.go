package latency

import (
	"fmt"
	"slices"
	"time"

	"github.com/relab/hotstuff"
)

const DefaultLocation = "default"

// Between returns the latency between locations a and b.
// If a or b are not valid locations, the function will panic.
func Between(a, b string) time.Duration {
	fromIdx, toIdx := slices.Index(allLocations, a), slices.Index(allLocations, b)
	return allLatencies[fromIdx][toIdx]
}

// ValidLocation returns the location if it is valid, or an error otherwise.
func ValidLocation(location string) (string, error) {
	if location == "" || location == DefaultLocation {
		return DefaultLocation, nil
	}
	if !slices.Contains(allLocations, location) {
		return "", fmt.Errorf("location %q not found", location)
	}
	return location, nil
}

// Matrix represents a latency matrix and locations.
type Matrix struct {
	enabled bool
	lm      [][]time.Duration
	locs    []string
}

// MatrixFrom returns the latencies between the given locations.
// If a location is not valid, the function will panic.
func MatrixFrom(locations []string) Matrix {
	locationIndices := make([]int, len(locations))
	for i, loc := range locations {
		if loc == DefaultLocation {
			// Default location has no latencies
			return Matrix{}
		}
		idx := slices.Index(allLocations, loc)
		if idx == -1 {
			panic(fmt.Sprintf("Location %q not found", loc))
		}
		locationIndices[i] = idx
	}
	newLatencies := make([][]time.Duration, len(locationIndices))
	for i, fromIdx := range locationIndices {
		newLatencies[i] = make([]time.Duration, len(locationIndices))
		for j, toIdx := range locationIndices {
			newLatencies[i][j] = allLatencies[fromIdx][toIdx]
		}
	}
	return Matrix{
		enabled: len(locations) > 0,
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
// If the ID is 0 or the latency matrix is not enabled, the function will return the default location.
// If the ID is out of range, the function will panic.
func (lm Matrix) Location(id hotstuff.ID) string {
	if id == 0 || !lm.enabled {
		return DefaultLocation
	} else if int(id) > len(lm.locs) {
		panic(fmt.Sprintf("ID %d out of range", id))
	}
	return lm.locs[id-1]
}

// Enabled returns true if a latency matrix was provided.
func (lm Matrix) Enabled() bool {
	return lm.enabled
}

// Delay sleeps for the duration of the latency between nodes a and b.
func (lm Matrix) Delay(a, b hotstuff.ID) {
	if !lm.Enabled() {
		return
	}
	delay := lm.Latency(a, b)
	time.Sleep(delay)
}
