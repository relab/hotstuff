package leaderrotation

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/network/netconfig"
)

const RoundRobinModuleName = "round-robin"

type roundRobin struct {
	configuration *netconfig.Config
}

// GetLeader returns the id of the leader in the given view
func (rr roundRobin) GetLeader(view hotstuff.View) hotstuff.ID {
	// TODO: does not support reconfiguration
	// assume IDs start at 1
	return chooseRoundRobin(view, rr.configuration.Len())
}

// NewRoundRobin returns a new round-robin leader rotation implementation.
func NewRoundRobin(configuration *netconfig.Config) modules.LeaderRotation {
	return &roundRobin{configuration: configuration}
}

func chooseRoundRobin(view hotstuff.View, numReplicas int) hotstuff.ID {
	return hotstuff.ID(view%hotstuff.View(numReplicas) + 1)
}
