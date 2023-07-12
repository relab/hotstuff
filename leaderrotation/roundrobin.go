package leaderrotation

import (
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
	leader := hotstuff.ID(1)
	for true {
		leader = chooseRoundRobin(view, rr.configuration.Len())
		replica, ok := rr.configuration.Replica(leader)
		if ok && replica.Active() {
			break
		}
		view += 1
	}
	return leader
}

// NewRoundRobin returns a new round-robin leader rotation implementation.
func NewRoundRobin() modules.LeaderRotation {
	return &roundRobin{}
}

func chooseRoundRobin(view hotstuff.View, numReplicas int) hotstuff.ID {
	return hotstuff.ID(view%hotstuff.View(numReplicas) + 1)
}
