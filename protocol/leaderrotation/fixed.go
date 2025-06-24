package leaderrotation

import (
	"github.com/relab/hotstuff"
)

const ModuleNameFixed = "fixed"

type Fixed struct {
	leader hotstuff.ID
}

// GetLeader returns the id of the leader in the given view
func (f *Fixed) GetLeader(_ hotstuff.View) hotstuff.ID {
	return f.leader
}

// NewFixed returns a new fixed-leader leader rotation implementation.
func NewFixed(
	leader hotstuff.ID,
) *Fixed {
	return &Fixed{
		leader: leader,
	}
}

var _ LeaderRotation = (*Fixed)(nil)
