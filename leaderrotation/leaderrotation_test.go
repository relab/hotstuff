package leaderrotation_test

import (
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/leaderrotation"
	"github.com/relab/hotstuff/modules"
)

type mockConfig struct {
	modules.Configuration
	len int
}

func (cfg *mockConfig) Len() int {
	return cfg.len
}

var _ modules.Configuration = (*mockConfig)(nil)

func TestRoundRobin(t *testing.T) {
	length := 4
	cycles := 3

	cfg := &mockConfig{len: length}
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
