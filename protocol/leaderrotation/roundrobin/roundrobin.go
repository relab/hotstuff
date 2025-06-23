package roundrobin

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/protocol/leaderrotation"
)

const ModuleName = "round-robin"

type RoundRobin struct {
	config *core.RuntimeConfig
}

// GetLeader returns the id of the leader in the given view
func (rr RoundRobin) GetLeader(view hotstuff.View) hotstuff.ID {
	// TODO: does not support reconfiguration
	// assume IDs start at 1
	return leaderrotation.ChooseRoundRobin(view, rr.config.ReplicaCount())
}

// NewRoundRobin returns a new round-robin leader rotation implementation.
func New(config *core.RuntimeConfig) *RoundRobin {
	return &RoundRobin{
		config: config,
	}
}

var _ leaderrotation.LeaderRotation = (*RoundRobin)(nil)
