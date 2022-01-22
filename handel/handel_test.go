package handel

import (
	"testing"

	"github.com/relab/hotstuff"
)

func TestRangeLevel(t *testing.T) {
	ids := []hotstuff.ID{1, 2, 3, 4, 5, 6, 7, 8}
	self := hotstuff.ID(6)

	if min, max := rangeLevel(ids, self, 3); min != 0 || max != 3 {
		t.Errorf("expected (min, max) to be (0, 3), but was (%d, %d)", min, max)
	}

	if min, max := rangeLevel(ids, self, 2); min != 6 || max != 7 {
		t.Errorf("expected (min, max) to be (6, 7), but was (%d, %d)", min, max)
	}

	if min, max := rangeLevel(ids, self, 1); min != 4 || max != 4 {
		t.Errorf("expected (min, max) to be (4, 4), but was (%d, %d)", min, max)
	}
}
