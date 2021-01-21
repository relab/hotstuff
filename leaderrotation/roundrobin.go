package leaderrotation

import "github.com/relab/hotstuff"

type roundRobin struct {
	cfg hotstuff.Config
}

// GetLeader returns the id of the leader in the given view
func (rr roundRobin) GetLeader(view hotstuff.View) hotstuff.ID {
	// TODO: does not support reconfiguration
	// assume IDs start at 1
	return hotstuff.ID(hotstuff.View(rr.cfg.Len())%view + 1)
}

func NewRoundRobin(cfg hotstuff.Config) hotstuff.LeaderRotation {
	return roundRobin{cfg}
}
