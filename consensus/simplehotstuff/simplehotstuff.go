// Package simplehotstuff implements a simplified version of the three-chain HotStuff protocol.
package simplehotstuff

import (
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/modules"
)

func init() {
	modules.RegisterModule("simplehotstuff", func() consensus.Rules {
		return New()
	})
}

// SimpleHotStuff implements a simplified version of the HotStuff algorithm.
//
// Based on the simplified algorithm described in the paper
// "Formal Verification of HotStuff" by Leander Jehl.
type SimpleHotStuff struct {
	mods *consensus.Modules

	locked *consensus.Block
}

// New returns a new SimpleHotStuff instance.
func New() consensus.Rules {
	return &SimpleHotStuff{
		locked: consensus.GetGenesis(),
	}
}

// InitConsensusModule gives the module a reference to the Modules object.
// It also allows the module to set module options using the OptionsBuilder.
func (hs *SimpleHotStuff) InitConsensusModule(mods *consensus.Modules, _ *consensus.OptionsBuilder) {
	hs.mods = mods
}

// VoteRule decides if the replica should vote for the given block.
func (hs *SimpleHotStuff) VoteRule(proposal consensus.ProposeMsg) bool {
	block := proposal.Block

	// Rule 1: can only vote in increasing rounds
	if block.View() < hs.mods.Synchronizer().View() {
		hs.mods.Logger().Info("VoteRule: block view too low")
		return false
	}

	parent, ok := hs.mods.BlockChain().Get(block.QuorumCert().BlockHash())
	if !ok {
		hs.mods.Logger().Info("VoteRule: missing parent block: ", block.QuorumCert().BlockHash())
		return false
	}

	// Rule 2: can only vote if parent's view is greater than or equal to locked block's view.
	if parent.View() < hs.locked.View() {
		hs.mods.Logger().Info("OnPropose: parent too old")
		return false
	}

	return true
}

// CommitRule decides if an ancestor of the block can be committed, and returns the ancestor, otherwise returns nil.
func (hs *SimpleHotStuff) CommitRule(block *consensus.Block) *consensus.Block {
	// will consider if the great-grandparent of the new block can be committed.
	p, ok := hs.mods.BlockChain().Get(block.QuorumCert().BlockHash())
	if !ok {
		return nil
	}

	gp, ok := hs.mods.BlockChain().Get(p.QuorumCert().BlockHash())
	if ok && gp.View() > hs.locked.View() {
		hs.locked = gp
		hs.mods.Logger().Debug("Locked: ", gp)
	} else if !ok {
		return nil
	}

	ggp, ok := hs.mods.BlockChain().Get(gp.QuorumCert().BlockHash())
	// we commit the great-grandparent of the block if its grandchild is certified,
	// which we already know is true because the new block contains the grandchild's certificate,
	// and if the great-grandparent's view + 2 equals the grandchild's view.
	if ok && ggp.View()+2 == p.View() {
		return ggp
	}
	return nil
}
