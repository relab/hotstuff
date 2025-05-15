package consensus

import "github.com/relab/hotstuff"

type Leader struct {
}

func NewLeader() *Leader {
	return &Leader{}
}

func Propose(_ hotstuff.ProposeMsg) {

}
