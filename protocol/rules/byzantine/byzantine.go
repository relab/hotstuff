// Package byzantine contains Byzantine consensus rules.
package byzantine

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/security/blockchain"
)

const (
	SilenceModuleName = "silence"
	ForkModuleName    = "fork"
)

type Silence struct {
	consensus.Ruleset
}

func (s *Silence) ProposeRule(_ hotstuff.View, _ hotstuff.QuorumCert, _ hotstuff.SyncInfo, _ *clientpb.Batch) (hotstuff.ProposeMsg, bool) {
	return hotstuff.ProposeMsg{}, false
}

// NewSilence returns a byzantine replica that will never propose.
func NewSilence(rules consensus.Ruleset) *Silence {
	return &Silence{Ruleset: rules}
}

type Fork struct {
	blockchain *blockchain.Blockchain
	config     *core.RuntimeConfig
	consensus.Ruleset
}

func (f *Fork) ProposeRule(view hotstuff.View, highQC hotstuff.QuorumCert, cert hotstuff.SyncInfo, cmd *clientpb.Batch) (proposal hotstuff.ProposeMsg, ok bool) {
	block, ok := f.blockchain.Get(highQC.BlockHash())
	if !ok {
		return proposal, false
	}
	parent, ok := f.blockchain.Get(block.Parent())
	if !ok {
		return proposal, false
	}
	grandparent, ok := f.blockchain.Get(parent.Hash())
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
	rules consensus.Ruleset,
	blockchain *blockchain.Blockchain,
	config *core.RuntimeConfig,
) *Fork {
	return &Fork{
		Ruleset:    rules,
		blockchain: blockchain,
		config:     config,
	}
}

var (
	_ consensus.Ruleset = (*Silence)(nil)
	_ consensus.Ruleset = (*Fork)(nil)
)
