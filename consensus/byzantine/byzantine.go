// Package byzantine contiains byzantine behaviors that can be applied to the consensus protocols.
package byzantine

import (
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/msg"
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

// InitModule gives the module a reference to the ConsensusCore object.
// It also allows the module to set module options using the OptionsBuilder.
func (s *silence) InitModule(mods *modules.ConsensusCore, opts *modules.OptionsBuilder) {
	if mod, ok := s.Rules.(modules.ConsensusModule); ok {
		mod.InitModule(mods, opts)
	}
}

func (s *silence) ProposeRule(_ *msg.SyncInfo, _ msg.Command) (*msg.Proposal, bool) {
	return &msg.Proposal{}, false
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
	mods *modules.ConsensusCore
	consensus.Rules
}

// InitModule gives the module a reference to the ConsensusCore object.
// It also allows the module to set module options using the OptionsBuilder.
func (f *fork) InitModule(mods *modules.ConsensusCore, opts *modules.OptionsBuilder) {
	f.mods = mods
	if mod, ok := f.Rules.(modules.ConsensusModule); ok {
		mod.InitModule(mods, opts)
	}
}

func (f *fork) ProposeRule(cert *msg.SyncInfo, cmd msg.Command) (proposal *msg.Proposal, ok bool) {
	parent, ok := f.mods.BlockChain().Get(f.mods.Synchronizer().LeafBlock().ParentHash())
	if !ok {
		return proposal, false
	}
	grandparent, ok := f.mods.BlockChain().Get(parent.GetBlockHash())
	if !ok {
		return proposal, false
	}

	proposal = &msg.Proposal{
		Block: msg.NewBlock(
			grandparent.GetBlockHash(),
			grandparent.QuorumCert(),
			cmd,
			f.mods.Synchronizer().View(),
			f.mods.ID(),
		),
	}
	if aggQC, ok := cert.AggQC(); f.mods.Options().ShouldUseAggQC() && ok {
		proposal.AggQC = aggQC
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
