package consensus

import (
	"github.com/relab/hotstuff/modules"
)

type ProposerOption func(*Proposer)

func OverrideProposeRule(impl modules.ProposeRuler) ProposerOption {
	return func(c *Proposer) {
		c.ruler = impl
	}
}
