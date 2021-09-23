package leaderrotation

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
)

type random struct {
	leader hotstuff.ID
}

//GetLeader returns the id of the leader in the given view
func (r random) GetLeader(_ consensus.View) hotstuff.ID {
	return r.leader
}

//NewRandom returns a new random leader rotation implementation
func NewRandom(leader hotstuff.ID) consensus.LeaderRotation {
	return random{leader}
}
