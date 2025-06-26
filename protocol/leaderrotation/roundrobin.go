package leaderrotation

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
)

const NameRoundRobin = "round-robin"

type RoundRobin struct {
	config *core.RuntimeConfig
}

// GetLeader returns the id of the leader in the given view
func (rr RoundRobin) GetLeader(view hotstuff.View) hotstuff.ID {
	// TODO: does not support reconfiguration
	// assume IDs start at 1
	return ChooseRoundRobin(view, rr.config.ReplicaCount())
}

// NewRoundRobin returns a new round-robin leader rotation implementation.
func NewRoundRobin(config *core.RuntimeConfig) *RoundRobin {
	return &RoundRobin{
		config: config,
	}
}

var _ LeaderRotation = (*RoundRobin)(nil)
