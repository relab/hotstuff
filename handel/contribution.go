package handel

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
)

type contribution struct {
	hash       consensus.Hash
	sender     hotstuff.ID
	level      int
	signature  consensus.QuorumSignature
	individual consensus.QuorumSignature
	verified   bool
	deferred   bool
	score      int
}
