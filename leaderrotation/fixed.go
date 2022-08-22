package leaderrotation

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/modules"
)

func init() {
	modules.RegisterModule("fixed", func() modules.LeaderRotation {
		return NewFixed(1)
	})
}

type fixed struct {
	leader hotstuff.ID
}

func (*fixed) New() fixed {
	return fixed{}
}

// GetLeader returns the id of the leader in the given view
func (f fixed) GetLeader(_ hotstuff.View) hotstuff.ID {
	return f.leader
}

// NewFixed returns a new fixed-leader leader rotation implementation.
func NewFixed(leader hotstuff.ID) modules.LeaderRotation {
	return fixed{leader: leader}
}
