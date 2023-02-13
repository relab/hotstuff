// Package byzantine contiains byzantine behaviors that can be applied to the consensus protocols.
package byzantine

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/modules"
)

func init() {
	modules.RegisterModule("silence", func() Byzantine { return &silence{} })
	modules.RegisterModule("fork", func() Byzantine { return &fork{} })
}

// Byzantine wraps a consensus rules implementation and alters its behavior.
type Byzantine interface {
	// Wrap wraps the rules and returns an altered rules implementation.
	Wrap(consensus.Rules) consensus.Rules
}

type silence struct {
	consensus.Rules
}

func (s *silence) InitModule(mods *modules.Core) {
	if mod, ok := s.Rules.(modules.Module); ok {
		mod.InitModule(mods)
	}
}

func (s *silence) ProposeRule(_ hotstuff.SyncInfo, _ hotstuff.Command) (hotstuff.ProposeMsg, bool) {
	return hotstuff.ProposeMsg{}, false
}

func (s *silence) Wrap(rules consensus.Rules) consensus.Rules {
	s.Rules = rules
	return s
}

// NewSilence returns a byzantine replica that will never propose.
func NewSilence(c consensus.Rules) consensus.Rules {
	return &silence{Rules: c}
}

type fork struct {
	blockChain   modules.BlockChain
	synchronizer modules.Synchronizer
	opts         *modules.Options
	consensus.Rules
}

func (f *fork) InitModule(mods *modules.Core) {
	mods.Get(
		&f.blockChain,
		&f.synchronizer,
		&f.opts,
	)

	if mod, ok := f.Rules.(modules.Module); ok {
		mod.InitModule(mods)
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
func NewFork(rules consensus.Rules) consensus.Rules {
	return &fork{Rules: rules}
}

func (f *fork) Wrap(rules consensus.Rules) consensus.Rules {
	f.Rules = rules
	return f
}
