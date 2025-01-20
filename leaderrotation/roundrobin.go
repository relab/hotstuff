package leaderrotation

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/modules"
)

func init() {
	modules.RegisterModule("round-robin", NewRoundRobin)
}

type roundRobin struct {
	configuration core.Configuration
}

func (rr *roundRobin) InitComponent(mods *core.Core) {
	rr.configuration = mods.Components().Configuration
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
	return hotstuff.ID(view%hotstuff.View(numReplicas) + 1)
}
