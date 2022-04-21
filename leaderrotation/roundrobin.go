package leaderrotation

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/msg"
)

func init() {
	modules.RegisterModule("round-robin", NewRoundRobin)
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
func (rr roundRobin) GetLeader(view msg.View) hotstuff.ID {
	// TODO: does not support reconfiguration
	// assume IDs start at 1
	return chooseRoundRobin(view, rr.mods.Configuration().Len())
}

// NewRoundRobin returns a new round-robin leader rotation implementation.
func NewRoundRobin() consensus.LeaderRotation {
	return &roundRobin{}
}

func chooseRoundRobin(view msg.View, numReplicas int) hotstuff.ID {
	return hotstuff.ID(view%msg.View(numReplicas) + 1)
}
