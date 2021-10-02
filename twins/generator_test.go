package twins

import (
	"reflect"
	"testing"
	"time"

	"github.com/relab/hotstuff/consensus"
)

func TestPartitionsGenerator(t *testing.T) {
	pg := newPartGen(8, 3)
	for p := pg.nextPartitions(); p != nil; p = pg.nextPartitions() {
		t.Log(p)
	}
}

func TestGenerator(t *testing.T) {
	g := NewGenerator(4, 1, 3, 8, 10*time.Millisecond, func() consensus.Consensus { return nil })
	g.Shuffle(time.Now().Unix())
	for i := 0; i < 1000; i++ {
		t.Log(g.NextScenario())
	}
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
