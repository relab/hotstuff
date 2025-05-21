package roundrobin

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol/leaderrotation"
	"github.com/relab/hotstuff/protocol/synchronizer/viewduration"
)

const ModuleName = "round-robin"

type roundRobin struct {
	config       *core.RuntimeConfig
	viewDuration modules.ViewDuration
}

func (rr roundRobin) ViewDuration() modules.ViewDuration {
	return rr.viewDuration
}

// GetLeader returns the id of the leader in the given view
func (rr roundRobin) GetLeader(view hotstuff.View) hotstuff.ID {
	// TODO: does not support reconfiguration
	// assume IDs start at 1
	return leaderrotation.ChooseRoundRobin(view, rr.config.ReplicaCount())
}

// NewRoundRobin returns a new round-robin leader rotation implementation.
func New(config *core.RuntimeConfig, params viewduration.Params) modules.LeaderRotation {
	return &roundRobin{
		config:       config,
		viewDuration: viewduration.NewDynamic(params),
	}
}
