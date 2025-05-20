package consensus

import (
	"github.com/relab/hotstuff/modules"
)

type Option func(*Consensus)

func OverrideProposeRule(impl modules.ProposeRuler) Option {
	return func(c *Consensus) {
		c.ruler = impl
	}
}
