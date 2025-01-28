// Package byzantine contiains byzantine behaviors that can be applied to the consensus protocols.
package byzantine

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/blockchain"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/modules"
)

const (
	SilenceModuleName = "silence"
	ForkModuleName    = "fork"
)

// Byzantine wraps a consensus rules implementation and alters its behavior.
type Byzantine interface {
	// Wrap wraps the rules and returns an altered rules implementation.
	Wrap() modules.ConsensusRules
}

type silence struct {
	modules.ConsensusRules
}

func (s *silence) ProposeRule(_ hotstuff.SyncInfo, _ hotstuff.Command) (hotstuff.ProposeMsg, bool) {
	return hotstuff.ProposeMsg{}, false
}

func (s *silence) Wrap() modules.ConsensusRules {
	return s
}

// NewSilence returns a byzantine replica that will never propose.
func NewSilence(rules modules.ConsensusRules) Byzantine {
	return &silence{ConsensusRules: rules}
}

type fork struct {
	blockChain *blockchain.BlockChain
	opts       *core.Options
	modules.ConsensusRules
}

func (f *fork) ProposeRule(view hotstuff.View, highQC hotstuff.QuorumCert, cert hotstuff.SyncInfo, cmd hotstuff.Command) (proposal hotstuff.ProposeMsg, ok bool) {
	block, ok := f.blockChain.Get(highQC.BlockHash())
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
			view,
			f.opts.ID(),
		),
	}
	if aggQC, ok := cert.AggQC(); f.opts.ShouldUseAggQC() && ok {
		proposal.AggregateQC = &aggQC
	}
	return proposal, true
}

// NewFork returns a byzantine replica that will try to fork the chain.
func NewFork(
	rules modules.ConsensusRules,

	blockChain *blockchain.BlockChain,
	opts *core.Options,
) Byzantine {
	return &fork{
		ConsensusRules: rules,

		blockChain: blockChain,
		opts:       opts,
	}
}

func (f *fork) Wrap() modules.ConsensusRules {
	return f
}
