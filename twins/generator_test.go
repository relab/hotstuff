package twins

import "testing"

func TestPartitionsGenerator(t *testing.T) {
	pg := newPartGen(4, 3)
	for p := pg.nextPartitions(); p != nil; p = pg.nextPartitions() {
		t.Log(p)
	}
}

func TestGenerator(t *testing.T) {
	g := NewGenerator(4, 1, 3, 8)
	for i := 0; i < 1000; i++ {
		t.Log(g.NextScenario())
	}
}
