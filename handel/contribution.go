package handel

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
)

type contribution struct {
	hash      consensus.Hash
	sender    hotstuff.ID
	level     int
	signature consensus.ThresholdSignature
	verified  bool
}

func (c contribution) isIndividual() bool {
	return c.signature.Participants().Len() == 1
}
