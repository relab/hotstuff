package latencies

import (
	"testing"

	"github.com/relab/hotstuff"
)

func TestLatencySymmetry(t *testing.T) {
	locations := Locations()
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
			latency := Latency(i, j)
			reverse := Latency(j, i)
			if latency != reverse {
				t.Errorf("Latency(%d, %d) != Latency(%d, %d) ==> %v != %v", i, j, j, i, latency, reverse)
			}
		}
	}
	for i := range locations {
		fromID := hotstuff.ID(i + 1)
		for j := range locations {
			toID := hotstuff.ID(j + 1)
			latency := LatencyID(fromID, toID)
			reverse := LatencyID(toID, fromID)
			if latency != reverse {
				t.Errorf("LatencyID(%d, %d) != LatencyID(%d, %d) ==> %v != %v", fromID, toID, toID, fromID, latency, reverse)
			}
		}
	}
}

func TestLatencies(t *testing.T) {
	locations := Locations()
	for _, location := range locations {
		latencies := Latencies(location)
		if len(latencies) != len(locations) {
			t.Errorf("len(Locations()) != len(Latencies(%s)) ==> %d != %d", location, len(locations), len(latencies))
		}
	}
}

func TestLocationName(t *testing.T) {
	for i, location := range Locations() {
		if location != LocationName(i) {
			t.Errorf("LocationName(%d) = %s, want %s", i, LocationName(i), location)
		}
	}
}
