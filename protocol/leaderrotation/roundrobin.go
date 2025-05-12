package leaderrotation

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core/globals"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol/synchronizer/viewduration"
)

const RoundRobinModuleName = "round-robin"

type roundRobin struct {
	globals      *globals.Globals
	viewDuration modules.ViewDuration
}

func (rr roundRobin) ViewDuration() modules.ViewDuration {
	return rr.viewDuration
}

// GetLeader returns the id of the leader in the given view
func (rr roundRobin) GetLeader(view hotstuff.View) hotstuff.ID {
	// TODO: does not support reconfiguration
	// assume IDs start at 1
	return chooseRoundRobin(view, rr.globals.ReplicaCount())
}

// NewRoundRobin returns a new round-robin leader rotation implementation.
func NewRoundRobin(globals *globals.Globals, params viewduration.Params) modules.LeaderRotation {
	return &roundRobin{
		globals:      globals,
		viewDuration: viewduration.NewDynamic(params),
	}
}

func chooseRoundRobin(view hotstuff.View, numReplicas int) hotstuff.ID {
	return hotstuff.ID(view%hotstuff.View(numReplicas) + 1)
}
