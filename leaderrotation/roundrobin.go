package leaderrotation

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/modules"
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
func (rr roundRobin) GetLeader(view consensus.View) hotstuff.ID {
	// TODO: does not support reconfiguration
	// assume IDs start at 1
	replica, ok := rr.mods.Configuration().Replica(rr.mods.ID())
	if ok && replica.IsOrchestrator() {
		return rr.GetOrchestratorLeader(view)
	}
	if view == 1 {
		return rr.mods.Configuration().GetLowestActiveId()
	}
	return rr.mods.Configuration().ActiveReplicaForIndex(int(view))
}

// NewRoundRobin returns a new round-robin leader rotation implementation.
func NewRoundRobin() consensus.LeaderRotation {
	return &roundRobin{}
}

func chooseRoundRobin(view consensus.View, numReplicas int) hotstuff.ID {
	return NewRoundRobin().GetLeader(view)
}

// GetOrchestratorLeader returns the leader for the view
func (rr roundRobin) GetOrchestratorLeader(view consensus.View) hotstuff.ID {
	return rr.mods.Configuration().OrchestratorForIndex(int(view))
}
