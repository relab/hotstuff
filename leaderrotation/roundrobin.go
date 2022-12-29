package leaderrotation

import (
	"math"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/modules"
)

func init() {
	modules.RegisterModule("round-robin", NewRoundRobin)
}

type roundRobin struct {
	configuration modules.Configuration
}

func (rr *roundRobin) InitModule(mods *modules.Core) {
	mods.Get(&rr.configuration)
}

// GetLeader returns the id of the leader in the given view
func (rr roundRobin) GetLeader(view hotstuff.View) hotstuff.ID {
	// TODO: does not support reconfiguration
	// assume IDs start at 1
	return chooseRoundRobin(view, rr.configuration.Len())
}

// NewRoundRobin returns a new round-robin leader rotation implementation.
func NewRoundRobin() modules.LeaderRotation {
	return &roundRobin{}
}

func chooseRoundRobin(view hotstuff.View, numReplicas int) hotstuff.ID {
	id := uint64(math.Ceil(float64(view) / 2.0))
	return hotstuff.ID((id % uint64(numReplicas)) + 1)
}
