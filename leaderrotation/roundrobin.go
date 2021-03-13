package leaderrotation

import "github.com/relab/hotstuff"

type roundRobin struct {
	mod *hotstuff.HotStuff
}

func (rr roundRobin) InitModule(hs *hotstuff.HotStuff) {
	rr.mod = hs
}

// GetLeader returns the id of the leader in the given view
func (rr roundRobin) GetLeader(view hotstuff.View) hotstuff.ID {
	// TODO: does not support reconfiguration
	// assume IDs start at 1
	return hotstuff.ID(view%hotstuff.View(rr.mod.Config().Len()) + 1)
}

// NewRoundRobin returns a new round-robin leader rotation implementation.
func NewRoundRobin() hotstuff.LeaderRotation {
	return roundRobin{}
}
