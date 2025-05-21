package proposer

import (
	"github.com/relab/hotstuff/modules"
)

type Option func(*Proposer)

func OverrideProposeRule(impl modules.ProposeRuler) Option {
	return func(c *Proposer) {
		c.ruler = impl
	}
}
