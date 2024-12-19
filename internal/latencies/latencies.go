package latencies

import (
	"slices"
	"time"

	"github.com/relab/hotstuff"
)

// TODO: determine how to map config.Config.Locations ([]string) to latencies.Locations

// Locations returns all locations.
func Locations() []string {
	return locations
}

// Latency returns the latency between from and to locations.
func Latency(from, to int) time.Duration {
	return latencies[from][to]
}

// LatencyCity returns the latency between from and to locations.
func LatencyCity(from, to string) time.Duration {
	fromIdx, toIdx := slices.Index(locations, from), slices.Index(locations, to)
	return latencies[fromIdx][toIdx]
}

// LatencyID returns the latency between from and to nodes.
func LatencyID(from, to hotstuff.ID) time.Duration {
	return latencies[from-1][to-1]
}

// TODO: write tests for these functions
// TODO: update latencygen to add city name as a /* City */ comment
// TODO: add a function to get the index of a city?? Necessary?
// TODO: check that the latencies are symmetric
// TODO: check that the latencies are correctly stored in ms units.
// TODO: check that the latencies are one-way latencies.
