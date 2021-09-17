package twins

import (
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
