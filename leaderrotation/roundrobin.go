package leaderrotation

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/modules"
)

func init() {
	modules.RegisterModule("round-robin", func() consensus.LeaderRotation {
		return NewRoundRobin()
	})
}

type roundRobin struct {
	mods *consensus.Modules
}

// InitConsensusModule gives the module a reference to the Modules object.
// It also allows the module to set module options using the OptionsBuilder.
func (rr *roundRobin) InitConsensusModule(mods *consensus.Modules, _ *consensus.OptionsBuilder) {
	rr.mods = mods
}

// GetLeader returns the id of the leader in the given view
func (rr roundRobin) GetLeader(view consensus.View) hotstuff.ID {
	// TODO: does not support reconfiguration
	// assume IDs start at 1
	return hotstuff.ID(view%consensus.View(rr.mods.Configuration().Len()) + 1)
}

// NewRoundRobin returns a new round-robin leader rotation implementation.
func NewRoundRobin() consensus.LeaderRotation {
	return &roundRobin{}
}
