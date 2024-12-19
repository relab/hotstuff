package latencies

import (
	"slices"
	"time"

	"github.com/relab/hotstuff"
)

// Locations returns all city locations.
func Locations() []string {
	return cities
}

// Latency returns the latency between from and to cities.
func Latency(from, to int) time.Duration {
	return latencies[from][to]
}

// LatencyCity returns the latency between from and to cities.
func LatencyCity(from, to string) time.Duration {
	fromIdx, toIdx := slices.Index(cities, from), slices.Index(cities, to)
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
