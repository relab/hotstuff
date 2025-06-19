package fixedleader

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/modules"
)

const ModuleName = "fixed"

type FixedLeader struct {
	leader hotstuff.ID
}

// GetLeader returns the id of the leader in the given view
func (f *FixedLeader) GetLeader(_ hotstuff.View) hotstuff.ID {
	return f.leader
}

// NewFixed returns a new fixed-leader leader rotation implementation.
func New(
	leader hotstuff.ID,
) *FixedLeader {
	return &FixedLeader{
		leader: leader,
	}
}

var _ modules.LeaderRotation = (*FixedLeader)(nil)
