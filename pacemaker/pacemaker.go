package pacemaker

import "github.com/relab/hotstuff"

type Pacemaker struct {
	hotstuff.Consensus
}

func New(hs hotstuff.Consensus) *Pacemaker {
	return &Pacemaker{hs}
}
