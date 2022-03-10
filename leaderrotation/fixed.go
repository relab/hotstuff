package leaderrotation

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/modules"
)

func init() {
	modules.RegisterModule("fixed", func() consensus.LeaderRotation {
		return NewFixed(1)
	})
}

type fixed struct {
	leader hotstuff.ID
	mods   *consensus.Modules
}

// InitConsensusModule gives the module a reference to the Modules object.
// It also allows the module to set module options using the OptionsBuilder.
func (f *fixed) InitConsensusModule(mods *consensus.Modules, _ *consensus.OptionsBuilder) {
	f.mods = mods
}

// GetLeader returns the id of the leader in the given view
func (f fixed) GetLeader(v consensus.View) hotstuff.ID {
	replica, ok := f.mods.Configuration().Replica(f.mods.ID())
	if ok && replica.IsOrchestrator() {
		return f.GetOrchestratorLeader(v)
	}
	return f.mods.Configuration().GetLowestActiveId()
}

// NewFixed returns a new fixed-leader leader rotation implementation.
func NewFixed(leader hotstuff.ID) consensus.LeaderRotation {
	return fixed{leader: leader}
}

// GetOrchestratorLeader returns the lowest leader
func (f fixed) GetOrchestratorLeader(_ consensus.View) hotstuff.ID {
	return f.mods.Configuration().GetLowestOrchestratorID()
}
