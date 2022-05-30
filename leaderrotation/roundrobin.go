package leaderrotation

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/modules"
)

func init() {
	modules.RegisterModule("round-robin", NewRoundRobin)
}

type roundRobin struct {
	mods *modules.ConsensusCore
}

// InitModule gives the module a reference to the ConsensusCore object.
// It also allows the module to set module options using the OptionsBuilder.
func (rr *roundRobin) InitModule(mods *modules.ConsensusCore, _ *modules.OptionsBuilder) {
	rr.mods = mods
}

// GetLeader returns the id of the leader in the given view
func (rr roundRobin) GetLeader(view hotstuff.View) hotstuff.ID {
	// TODO: does not support reconfiguration
	// assume IDs start at 1
	return chooseRoundRobin(view, rr.mods.Configuration().Len())
}

// NewRoundRobin returns a new round-robin leader rotation implementation.
func NewRoundRobin() modules.LeaderRotation {
	return &roundRobin{}
}

func chooseRoundRobin(view hotstuff.View, numReplicas int) hotstuff.ID {
	return hotstuff.ID(view%hotstuff.View(numReplicas) + 1)
}
