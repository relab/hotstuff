// Package byzantine contains Byzantine consensus rules.
package byzantine

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/security/blockchain"
)

const (
	SilenceModuleName = "silence"
	ForkModuleName    = "fork"
)

type silence struct {
	modules.HotstuffRuleset
}

func (s *silence) ProposeRule(_ hotstuff.View, _ hotstuff.QuorumCert, _ hotstuff.SyncInfo, _ hotstuff.Command) (hotstuff.ProposeMsg, bool) {
	return hotstuff.ProposeMsg{}, false
}

// NewSilence returns a byzantine replica that will never propose.
func NewSilence(rules modules.HotstuffRuleset) modules.HotstuffRuleset {
	return &silence{HotstuffRuleset: rules}
}

type fork struct {
	blockChain *blockchain.BlockChain
	config     *core.RuntimeConfig
	modules.HotstuffRuleset
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
		ID: f.config.ID(),
		Block: hotstuff.NewBlock(
			grandparent.Hash(),
			grandparent.QuorumCert(),
			cmd,
			view,
			f.config.ID(),
		),
	}
	if aggQC, ok := cert.AggQC(); f.config.HasAggregateQC() && ok {
		proposal.AggregateQC = &aggQC
	}
	return proposal, true
}

// NewFork returns a byzantine replica that will try to fork the chain.
func NewFork(
	rules modules.HotstuffRuleset,
	blockChain *blockchain.BlockChain,
	config *core.RuntimeConfig,
) modules.HotstuffRuleset {
	return &fork{
		HotstuffRuleset: rules,
		blockChain:      blockChain,
		config:          config,
	}
}

var (
	_ modules.HotstuffRuleset = (*silence)(nil)
	_ modules.ProposeRuler    = (*silence)(nil)
	_ modules.HotstuffRuleset = (*fork)(nil)
	_ modules.ProposeRuler    = (*fork)(nil)
)
