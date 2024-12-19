package latencies

import (
	"testing"

	"github.com/relab/hotstuff"
)

func TestLatencySymmetry(t *testing.T) {
	for _, fromLoc := range locations {
		for _, toLoc := range locations {
			latency := LatencyCity(fromLoc, toLoc)
			reverse := LatencyCity(toLoc, fromLoc)
			if latency != reverse {
				t.Errorf("LatencyCity(%s, %s) != LatencyCity(%s, %s) ==> %v != %v", fromLoc, toLoc, toLoc, fromLoc, latency, reverse)
			}
		}
	}
	for i := range locations {
		for j := range locations {
			latency := latencies[i][j]
			reverse := latencies[j][i]
			if latency != reverse {
				t.Errorf("Latency(%d, %d) != Latency(%d, %d) ==> %v != %v", i, j, j, i, latency, reverse)
			}
		}
	}
}

func TestLatenciesFrom(t *testing.T) {
	locations := []string{"Melbourne", "Toronto", "Prague", "Paris", "Tokyo", "Amsterdam", "Auckland", "Moscow", "Stockholm", "London"}
	lm := LatenciesFrom(locations)
	if len(lm) != len(locations) {
		t.Errorf("len(LatenciesFrom(%v)) = %d, want %d", locations, len(lm), len(locations))
	}
	for i, fromLoc := range locations {
		id1 := hotstuff.ID(i + 1)
		for j, toLoc := range locations {
			id2 := hotstuff.ID(j + 1)
			// We can lookup the latency between location names using the global latencies matrix
			// or by using the LatencyID method on the LatencyMatrix created by LatenciesFrom.
			cityLatency := LatencyCity(fromLoc, toLoc)
			lmLatency := lm.LatencyID(id1, id2)
			if cityLatency != lmLatency {
				t.Errorf("LatencyCity(%s, %s) != lm.LatencyID(%d, %d) ==> %v != %v", fromLoc, toLoc, id1, id2, cityLatency, lmLatency)
			}
		}
	}
}
