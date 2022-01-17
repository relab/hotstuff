package handel

import (
	"reflect"
	"testing"

	"github.com/relab/hotstuff"
)

func TestCreatePartitions(t *testing.T) {
	want := [][]hotstuff.ID{
		{},
		{5},
		{7, 8},
		{1, 2, 3, 4},
	}

	got := createPartitions(3, []hotstuff.ID{1, 2, 3, 4, 5, 6, 7, 8}, 6)

	if !reflect.DeepEqual(got, want) {
		t.Error("got: ", got, "want: ", want)
	}
}
