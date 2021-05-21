package leaderrotation

import (
	"github.com/relab/hotstuff/consensus"
)

type fixed struct {
	leader consensus.ID
}

// GetLeader returns the id of the leader in the given view
func (f fixed) GetLeader(_ consensus.View) consensus.ID {
	return f.leader
}

// NewFixed returns a new fixed-leader leader rotation implementation.
func NewFixed(leader consensus.ID) consensus.LeaderRotation {
	return fixed{leader}
}
