package leaderrotation

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/modules"
)

func init() {
	modules.RegisterModule("fixed", func() consensus.LeaderRotation {
		return NewFixed(1)
	})
}

type fixed struct {
	leader hotstuff.ID
}

// GetLeader returns the id of the leader in the given view
func (f fixed) GetLeader(_ consensus.View) hotstuff.ID {
	return f.leader
}


// NewFixed returns a new fixed-leader leader rotation implementation.
func NewFixed(leader hotstuff.ID) consensus.LeaderRotation {
	return fixed{leader}
}
