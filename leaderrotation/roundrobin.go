package leaderrotation

import (
	"github.com/relab/hotstuff/consensus"
)

type roundRobin struct {
	mod *consensus.Modules
}

func (rr *roundRobin) InitModule(hs *consensus.Modules, _ *consensus.OptionsBuilder) {
	rr.mod = hs
}

// GetLeader returns the id of the leader in the given view
func (rr roundRobin) GetLeader(view consensus.View) consensus.ID {
	// TODO: does not support reconfiguration
	// assume IDs start at 1
	return consensus.ID(view%consensus.View(rr.mod.Configuration().Len()) + 1)
}

// NewRoundRobin returns a new round-robin leader rotation implementation.
func NewRoundRobin() consensus.LeaderRotation {
	return &roundRobin{}
}
