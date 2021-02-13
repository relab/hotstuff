package leaderrotation

import "github.com/relab/hotstuff"

type fixed struct {
	leader hotstuff.ID
}

// GetLeader returns the id of the leader in the given view
func (f fixed) GetLeader(_ hotstuff.View) hotstuff.ID {
	return f.leader
}

// NewFixed returns a new fixed-leader leader rotation implementation.
func NewFixed(leader hotstuff.ID) hotstuff.LeaderRotation {
	return fixed{leader}
}
