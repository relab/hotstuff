package handel

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
)

type contribution struct {
	sender    hotstuff.ID
	level     int
	single    bool
	signature consensus.ThresholdSignature
}
