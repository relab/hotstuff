package consensus

import (
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol/kauri"
)

type Option func(*Consensus)

// WithKauri overrides the vote dissemination method with Kauri's implementation.
func WithKauri(disseminator *kauri.Kauri) Option {
	return func(cs *Consensus) {
		cs.disseminator = disseminator
	}
}

func OverrideProposeRule(impl modules.ProposeRuler) Option {
	return func(c *Consensus) {
		c.ruler = impl
	}
}
