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

// Latencies returns the latencies to other locations for a given location.
func Latencies(location string) []time.Duration {
	locIndex := slices.Index(locations, location)
	return latencies[locIndex]
}

// LocationName returns the location name at the given index.
func LocationName(index int) string {
	return locations[index]
}
