package twins

import (
	"reflect"
	"testing"
	"time"

	"github.com/relab/hotstuff/logging"
)

func TestPartitionsGenerator(t *testing.T) {
	partitions := genPartitionScenarios([]NodeID{{1, 1}, {1, 2}}, []NodeID{{2, 3}, {3, 4}, {4, 5}}, 3, 1)
	for p := range partitions {
		t.Log(partitions[p])
	}
}

func TestGenerator(t *testing.T) {
	g := NewGenerator(logging.New(""), 4, 1, 3, 8)
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
