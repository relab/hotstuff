// Package byzantine contiains byzantine behaviors that can be applied to the consensus protocols.
package byzantine

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/modules"
)

func init() {
	modules.RegisterModule("silence", func() Byzantine { return &silence{} })
	modules.RegisterModule("fork", func() Byzantine { return &fork{} })
}

// Byzantine wraps a consensus rules implementation and alters its behavior.
type Byzantine interface {
	// Wrap wraps the rules and returns an altered rules implementation.
	Wrap(modules.Rules) modules.Rules
}

type silence struct {
	modules.Rules
}

func (s *silence) InitComponent(mods *core.Core) {
	if mod, ok := s.Rules.(core.Component); ok {
		mod.InitComponent(mods)
	}
}

func (s *silence) ProposeRule(_ hotstuff.SyncInfo, _ hotstuff.Command) (hotstuff.ProposeMsg, bool) {
	return hotstuff.ProposeMsg{}, false
}

func (s *silence) Wrap(rules modules.Rules) modules.Rules {
	s.Rules = rules
	return s
}

// NewSilence returns a byzantine replica that will never propose.
func NewSilence(c modules.Rules) modules.Rules {
	return &silence{Rules: c}
}

type fork struct {
	blockChain   core.BlockChain
	synchronizer core.Synchronizer
	opts         *core.Options
	modules.Rules
}

func (f *fork) InitComponent(mods *core.Core) {
	mods.Get(
		&f.blockChain,
		&f.synchronizer,
		&f.opts,
	)

	if mod, ok := f.Rules.(core.Component); ok {
		mod.InitComponent(mods)
	}
}

func (f *fork) ProposeRule(cert hotstuff.SyncInfo, cmd hotstuff.Command) (proposal hotstuff.ProposeMsg, ok bool) {
	block, ok := f.blockChain.Get(f.synchronizer.HighQC().BlockHash())
	if !ok {
		return proposal, false
	}
	parent, ok := f.blockChain.Get(block.Parent())
	if !ok {
		return proposal, false
	}
	grandparent, ok := f.blockChain.Get(parent.Hash())
	if !ok {
		return proposal, false
	}

	proposal = hotstuff.ProposeMsg{
		ID: f.opts.ID(),
		Block: hotstuff.NewBlock(
			grandparent.Hash(),
			grandparent.QuorumCert(),
			cmd,
			f.synchronizer.View(),
			f.opts.ID(),
		),
	}
	if aggQC, ok := cert.AggQC(); f.opts.ShouldUseAggQC() && ok {
		proposal.AggregateQC = &aggQC
	}
	return proposal, true
}

// NewFork returns a byzantine replica that will try to fork the chain.
func NewFork(rules modules.Rules) modules.Rules {
	return &fork{Rules: rules}
}

func (f *fork) Wrap(rules modules.Rules) modules.Rules {
	f.Rules = rules
	return f
}
