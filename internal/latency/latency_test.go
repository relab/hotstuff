package latency

import (
	"testing"

	"github.com/relab/hotstuff"
)

func TestLatencySymmetry(t *testing.T) {
	for _, fromLoc := range allLocations {
		for _, toLoc := range allLocations {
			latency := Between(fromLoc, toLoc)
			reverse := Between(toLoc, fromLoc)
			if latency != reverse {
				t.Errorf("LatencyCity(%s, %s) != LatencyCity(%s, %s) ==> %v != %v", fromLoc, toLoc, toLoc, fromLoc, latency, reverse)
			}
		}
	}
	for i := range allLocations {
		for j := range allLocations {
			latency := allLatencies[i][j]
			reverse := allLatencies[j][i]
			if latency != reverse {
				t.Errorf("Latency(%d, %d) != Latency(%d, %d) ==> %v != %v", i, j, j, i, latency, reverse)
			}
		}
	}
}

func TestLatenciesFrom(t *testing.T) {
	locations := []string{"Melbourne", "Toronto", "Prague", "Paris", "Tokyo", "Amsterdam", "Auckland", "Moscow", "Stockholm", "London"}
	xm := Matrix{}
	if xm.Enabled() {
		t.Errorf("LatencyMatrix{}.Enabled() = true, want false")
	}
	lm := MatrixFrom(locations)
	if !lm.Enabled() {
		t.Errorf("LatenciesFrom(%v).Enabled() = false, want true", locations)
	}
	if len(lm.lm) != len(locations) {
		t.Errorf("len(LatenciesFrom(%v)) = %d, want %d", locations, len(lm.lm), len(locations))
	}
	for i, fromLoc := range locations {
		id1 := hotstuff.ID(i + 1)
		for j, toLoc := range locations {
			id2 := hotstuff.ID(j + 1)
			// We can lookup the latency between location names using the global latencies matrix
			// or by using the Latency method on the LatencyMatrix created by LatenciesFrom.
			locLatency := Between(fromLoc, toLoc)
			lmLatency := lm.Latency(id1, id2)
			if locLatency != lmLatency {
				t.Errorf("Latency(%s, %s) != lm.LatencyID(%d, %d) ==> %v != %v", fromLoc, toLoc, id1, id2, locLatency, lmLatency)
			}
		}
	}
}
