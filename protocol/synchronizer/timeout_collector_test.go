package synchronizer

import (
	"maps"
	"slices"
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/internal/test"
)

func TestTimeoutQuorum(t *testing.T) {
	tests := []struct {
		name         string
		n            int
		timeouts     []hotstuff.TimeoutMsg
		wantTimeouts []hotstuff.TimeoutMsg
		wantQuorum   []bool
	}{
		{"NoQuorumNil", 4, nil, nil, []bool{false}},
		{"NoQuorum0", 4, []hotstuff.TimeoutMsg{}, nil, []bool{false}},
		{"NoQuorum1", 4, []hotstuff.TimeoutMsg{{View: 1, ID: 1}}, nil, []bool{false}},
		{"NoQuorum2", 4, []hotstuff.TimeoutMsg{{View: 1, ID: 1}, {View: 1, ID: 2}}, nil, []bool{false, false}},
		{"NoQuorum3Duplicates", 4, []hotstuff.TimeoutMsg{{View: 1, ID: 1}, {View: 1, ID: 2}, {View: 1, ID: 2}}, nil, []bool{false, false, false}},
		{"Quorum3", 4, []hotstuff.TimeoutMsg{{View: 1, ID: 1}, {View: 1, ID: 2}, {View: 1, ID: 3}}, []hotstuff.TimeoutMsg{{View: 1, ID: 1}, {View: 1, ID: 2}, {View: 1, ID: 3}}, []bool{false, false, true}},
		{"Quorum3Duplicates1", 4, []hotstuff.TimeoutMsg{{View: 1, ID: 1}, {View: 1, ID: 2}, {View: 1, ID: 2}, {View: 1, ID: 3}}, []hotstuff.TimeoutMsg{{View: 1, ID: 1}, {View: 1, ID: 2}, {View: 1, ID: 3}}, []bool{false, false, false, true}},
		{"Quorum3Duplicates2", 4, []hotstuff.TimeoutMsg{{View: 1, ID: 3}, {View: 1, ID: 3}, {View: 1, ID: 2}, {View: 1, ID: 3}, {View: 1, ID: 2}, {View: 1, ID: 1}}, []hotstuff.TimeoutMsg{{View: 1, ID: 1}, {View: 1, ID: 2}, {View: 1, ID: 3}}, []bool{false, false, false, false, false, true}},
		{"Quorum3Duplicates3", 4, []hotstuff.TimeoutMsg{{View: 1, ID: 1}, {View: 1, ID: 2}, {View: 1, ID: 3}, {View: 1, ID: 3}, {View: 1, ID: 2}, {View: 1, ID: 1}}, []hotstuff.TimeoutMsg{{View: 1, ID: 1}, {View: 1, ID: 2}, {View: 1, ID: 3}}, []bool{false, false, true, false, false, true}}, // TODO(meling): Should we get a new quorum indication here?
	}

	for _, tt := range tests {
		config := core.NewRuntimeConfig(1, nil)
		for i := range tt.n { // to set the quorum size
			config.AddReplica(&hotstuff.ReplicaInfo{
				ID: hotstuff.ID(i + 1),
			})
		}
		for _, ds := range []string{"MapMap", "MapSlice", "Slice"} {
			name := ds + "/" + tt.name
			s := newTimeoutStore(t, ds, tt.n, config)
			t.Run(name, func(t *testing.T) {
				for i, timeout := range tt.timeouts {
					got, quorum := s.add(timeout)
					if quorum != tt.wantQuorum[i] {
						t.Errorf("timeoutQuorum() quorum = %v, want %v", quorum, tt.wantQuorum[i])
						t.Logf(" got timeouts: %v", got)
						t.Logf("want timeouts: %v", tt.timeouts)
					}
					if quorum && len(got) != len(tt.wantTimeouts) {
						t.Errorf("timeoutQuorum() got = %v, want %v", got, tt.wantTimeouts)
					}
					if !quorum && len(got) != 0 {
						t.Errorf("timeoutQuorum() got = %v, want empty slice", got)
					}
					slices.SortFunc(got, func(a, b hotstuff.TimeoutMsg) int {
						if a.View != b.View {
							return int(a.View) - int(b.View)
						}
						return int(a.ID) - int(b.ID)
					})
					if quorum && !slices.Equal(got, tt.wantTimeouts) {
						t.Errorf("timeoutQuorum() got = %v, want %v", got, tt.wantTimeouts)
					}
				}
			})
		}
	}
}

type timeoutStorage interface {
	add(hotstuff.TimeoutMsg) ([]hotstuff.TimeoutMsg, bool)
}

func newTimeoutStore(t *testing.T, ds string, n int, config *core.RuntimeConfig) (s timeoutStorage) {
	switch ds {
	case "MapMap":
		s = &timeoutMapMap{
			timeoutsByView: make(map[hotstuff.View]map[hotstuff.ID]hotstuff.TimeoutMsg),
			quorumSize:     hotstuff.QuorumSize(n),
		}
	case "MapSlice":
		s = &timeoutMapSlice{
			timeoutsByView: make(map[hotstuff.View][]hotstuff.TimeoutMsg),
			replicaCount:   n,
			quorumSize:     hotstuff.QuorumSize(n),
		}
	case "Slice":
		s = newTimeoutCollector(config)
	default:
		t.Fatalf("unknown data structure: %s", ds)
	}
	return s
}

func BenchmarkTimeoutQuorum(b *testing.B) {
	bench := []struct {
		n   int
		dup int
	}{
		{n: 4, dup: 0}, // f=1
		{n: 4, dup: 1},
		{n: 4, dup: 2},
		{n: 4, dup: 3},
		{n: 7, dup: 0}, // f=2
		{n: 7, dup: 5},
		{n: 25, dup: 0}, // f=8
		{n: 25, dup: 5},
		{n: 25, dup: 10},
		{n: 25, dup: 20},
		{n: 100, dup: 0}, // f=33
		{n: 100, dup: 50},
		{n: 100, dup: 75},
	}
	for _, bc := range bench {
		// prepare timeouts
		timeouts := make([]hotstuff.TimeoutMsg, 0, bc.n+bc.dup)
		dup := bc.dup
		for i := range bc.n {
			timeout := hotstuff.TimeoutMsg{View: 1, ID: hotstuff.ID(i)}
			timeouts = append(timeouts, timeout)
			if dup > 0 {
				timeouts = append(timeouts, timeout)
				dup--
			}
		}
		params := test.Name([]string{"n", "timeouts"}, bc.n, len(timeouts))

		b.Run("MapMap/"+params, func(b *testing.B) {
			s := &timeoutMapMap{
				timeoutsByView: make(map[hotstuff.View]map[hotstuff.ID]hotstuff.TimeoutMsg),
				quorumSize:     hotstuff.QuorumSize(bc.n),
			}
			for b.Loop() {
				for _, timeout := range timeouts {
					s.add(timeout)
				}
			}
		})

		b.Run("MapSlice/"+params, func(b *testing.B) {
			s := &timeoutMapSlice{
				timeoutsByView: make(map[hotstuff.View][]hotstuff.TimeoutMsg),
				replicaCount:   bc.n,
				quorumSize:     hotstuff.QuorumSize(bc.n),
			}
			for b.Loop() {
				for _, timeout := range timeouts {
					s.add(timeout)
				}
			}
		})

		b.Run("Slice/"+params, func(b *testing.B) {
			config := core.NewRuntimeConfig(1, nil)
			for i := range bc.n { // to set the quorum size
				config.AddReplica(&hotstuff.ReplicaInfo{
					ID: hotstuff.ID(i + 1),
				})
			}
			s := newTimeoutCollector(config)
			for b.Loop() {
				for _, timeout := range timeouts {
					s.add(timeout)
				}
			}
		})
	}
}

// timeoutMapMap is a double map structure that stores timeouts by view and by replica ID.
// This is only included for benchmarking purposes, as it is not used in the actual synchronizer.
type timeoutMapMap struct {
	timeoutsByView map[hotstuff.View]map[hotstuff.ID]hotstuff.TimeoutMsg
	quorumSize     int
}

// add returns true if a quorum of timeouts has been collected for the view of given timeout message.
func (s *timeoutMapMap) add(timeout hotstuff.TimeoutMsg) ([]hotstuff.TimeoutMsg, bool) {
	timeoutsByReplica, ok := s.timeoutsByView[timeout.View]
	if !ok {
		timeoutsByReplica = make(map[hotstuff.ID]hotstuff.TimeoutMsg)
		s.timeoutsByView[timeout.View] = timeoutsByReplica
	}
	if _, ok := timeoutsByReplica[timeout.ID]; !ok {
		timeoutsByReplica[timeout.ID] = timeout
	}
	if len(timeoutsByReplica) < s.quorumSize {
		return nil, false
	}
	timeoutList := slices.Collect(maps.Values(timeoutsByReplica))
	delete(s.timeoutsByView, timeout.View)
	return timeoutList, true
}

// timeoutMapSlice is a map structure that stores timeouts by view, using a slice for each view.
// This is only included for benchmarking purposes, as it is not used in the actual synchronizer.
type timeoutMapSlice struct {
	timeoutsByView map[hotstuff.View][]hotstuff.TimeoutMsg
	replicaCount   int
	quorumSize     int
}

// add returns true if a quorum of timeouts has been collected for the view of given timeout message.
func (s *timeoutMapSlice) add(timeout hotstuff.TimeoutMsg) ([]hotstuff.TimeoutMsg, bool) {
	timeoutsByReplica, ok := s.timeoutsByView[timeout.View]
	if !ok {
		timeoutsByReplica = make([]hotstuff.TimeoutMsg, 0, s.replicaCount)
		s.timeoutsByView[timeout.View] = timeoutsByReplica
	}

	// ignore this timeout if we already have a timeout from this replica
	if slices.ContainsFunc(timeoutsByReplica, func(t hotstuff.TimeoutMsg) bool { return t.ID == timeout.ID }) {
		return nil, false
	}
	timeoutsByReplica = append(timeoutsByReplica, timeout)
	s.timeoutsByView[timeout.View] = timeoutsByReplica

	if len(timeoutsByReplica) < s.quorumSize {
		return nil, false
	}

	// remove the timeouts for this view from the map, as we have a quorum
	// and we don't need to keep them around anymore
	delete(s.timeoutsByView, timeout.View)
	return timeoutsByReplica, true
}
