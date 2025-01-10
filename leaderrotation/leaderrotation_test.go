package leaderrotation_test

import (
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/leaderrotation"
	"github.com/relab/hotstuff/modules"
)

type mockConfiguration struct {
	modules.Configuration
	len int
}

func (cfg *mockConfiguration) Len() int {
	return cfg.len
}

var _ modules.Configuration = (*mockConfiguration)(nil)

func TestRoundRobin(t *testing.T) {
	length := 4
	cycles := 100

	cfg := &mockConfiguration{len: length}
	rr := leaderrotation.NewRoundRobin()

	builder := modules.NewBuilder(hotstuff.ID(1), nil)
	builder.Add(
		cfg,
		rr,
	)

	builder.Build()

	checkLeader := func(view hotstuff.View) bool {
		leader := rr.GetLeader(view)
		expectedLeader := hotstuff.ID(view%hotstuff.View(length)) + 1
		return leader == expectedLeader
	}

	view := hotstuff.View(1)
	for range length * cycles {
		if !checkLeader(view) {
			t.Fail()
		}
		view++
	}
}

func TestFixed(t *testing.T) {
	expectedLeader := hotstuff.ID(1)
	views := 100

	fixed := leaderrotation.NewFixed(expectedLeader)

	view := hotstuff.View(1)
	for range views {
		if fixed.GetLeader(view) != expectedLeader {
			t.Fail()
		}
		view++
	}
}
