package twins

import (
	"reflect"
	"testing"
	"time"

	"github.com/relab/hotstuff/core/logging"
)

func TestPartitionsGenerator(t *testing.T) {
	partitions := genPartitionScenarios([]NodeID{{1, 1}, {1, 2}}, []NodeID{{2, 3}, {3, 4}, {4, 5}}, 3, 1)
	for p := range partitions {
		t.Log(partitions[p])
	}
}

func TestGenerator(t *testing.T) {
	g := NewGenerator(logging.New(""), Settings{
		NumNodes:   4,
		NumTwins:   1,
		Partitions: 3,
		Views:      8,
	})
	g.Shuffle(time.Now().Unix())
	t.Log(g.NextScenario())
}

func TestPartitionSizes(t *testing.T) {
	want := [][]uint8{
		{6, 0, 0, 0},
		{5, 1, 0, 0},
		{4, 2, 0, 0},
		{4, 1, 1, 0},
		{3, 3, 0, 0},
		{3, 2, 1, 0},
		{3, 1, 1, 1},
		{2, 2, 2, 0},
		{2, 2, 1, 1},
	}
	got := genPartitionSizes(6, 4, 1)

	if !reflect.DeepEqual(got, want) {
		for i := range got {
			t.Log(got[i])
		}
		t.Error("did not get the expected result")
	}
}

func TestAssignNodeIDs(t *testing.T) {
	tests := []struct {
		name      string
		numNodes  uint8
		numTwins  uint8
		wantNodes []NodeID
		wantTwins []NodeID
	}{
		{"no twins", 3, 0, []NodeID{{1, 0}, {2, 0}, {3, 0}}, nil},
		{"one twin pair", 2, 1, []NodeID{{2, 0}}, []NodeID{{1, 1}, {1, 2}}},
		{"multiple twin pairs", 4, 2, []NodeID{{3, 0}, {4, 0}}, []NodeID{{1, 1}, {1, 2}, {2, 1}, {2, 2}}},
		{"all twins", 2, 2, nil, []NodeID{{1, 1}, {1, 2}, {2, 1}, {2, 2}}},
		{"edge case: zero nodes", 0, 0, nil, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNodes, gotTwins := assignNodeIDs(tt.numNodes, tt.numTwins)
			
			if !reflect.DeepEqual(gotNodes, tt.wantNodes) {
				t.Errorf("assignNodeIDs() gotNodes = %v, want %v", gotNodes, tt.wantNodes)
			}
			
			if !reflect.DeepEqual(gotTwins, tt.wantTwins) {
				t.Errorf("assignNodeIDs() gotTwins = %v, want %v", gotTwins, tt.wantTwins)
			}
		})
	}
}
