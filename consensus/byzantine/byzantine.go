// Package byzantine contiains byzantine behaviors that can be applied to the consensus protocols.
package byzantine

import (
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

// InitConsensusModule gives the module a reference to the Modules object.
// It also allows the module to set module options using the OptionsBuilder.
func (s *silence) InitConsensusModule(mods *consensus.Modules, opts *consensus.OptionsBuilder) {
	if mod, ok := s.Rules.(consensus.Module); ok {
		mod.InitConsensusModule(mods, opts)
	}
}

func (s *silence) ProposeRule(_ consensus.SyncInfo, _ consensus.Command) (consensus.ProposeMsg, bool) {
	return consensus.ProposeMsg{}, false
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
	mods *consensus.Modules
	consensus.Rules
}

// InitConsensusModule gives the module a reference to the Modules object.
// It also allows the module to set module options using the OptionsBuilder.
func (f *fork) InitConsensusModule(mods *consensus.Modules, opts *consensus.OptionsBuilder) {
	f.mods = mods
	if mod, ok := f.Rules.(consensus.Module); ok {
		mod.InitConsensusModule(mods, opts)
	}
}

func (f *fork) ProposeRule(cert consensus.SyncInfo, cmd consensus.Command) (proposal consensus.ProposeMsg, ok bool) {
	parent, ok := f.mods.BlockChain().Get(f.mods.Synchronizer().LeafBlock().Parent())
	if !ok {
		return proposal, false
	}
	grandparent, ok := f.mods.BlockChain().Get(parent.Hash())
	if !ok {
		return proposal, false
	}

	proposal = consensus.ProposeMsg{
		ID: f.mods.ID(),
		Block: consensus.NewBlock(
			grandparent.Hash(),
			grandparent.QuorumCert(),
			cmd,
			f.mods.Synchronizer().View(),
			f.mods.ID(),
		),
	}
	if aggQC, ok := cert.AggQC(); f.mods.Options().ShouldUseAggQC() && ok {
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
