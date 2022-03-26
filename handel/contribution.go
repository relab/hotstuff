package handel

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
)

type contribution struct {
	hash      consensus.Hash
	sender    hotstuff.ID
	level     int
	signature consensus.QuorumSignature
	verified  bool
	deferred  bool
	score     int
}

func (c contribution) isIndividual() bool {
	return c.signature.Participants().Len() == 1
}
