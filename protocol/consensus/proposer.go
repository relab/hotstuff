package consensus

import "github.com/relab/hotstuff"

type Proposer struct {
}

func NewProposer() *Proposer {
	return &Proposer{}
}

func Propose(proposal hotstuff.ProposeMsg) {

}
