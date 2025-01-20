// Package simplehotstuff implements a simplified version of the three-chain HotStuff protocol.
package simplehotstuff

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/modules"
)

func init() {
	modules.RegisterModule("simplehotstuff", New)
}

// SimpleHotStuff implements a simplified version of the HotStuff algorithm.
//
// Based on the simplified algorithm described in the paper
// "Formal Verification of HotStuff" by Leander Jehl.
type SimpleHotStuff struct {
	comps core.ComponentList

	locked *hotstuff.Block
}

// New returns a new SimpleHotStuff instance.
func New() modules.Rules {
	return &SimpleHotStuff{
		locked: hotstuff.GetGenesis(),
	}
}

// InitComponent initializes the module.
func (hs *SimpleHotStuff) InitComponent(mods *core.Core) {
	hs.comps = mods.Components()
}

// VoteRule decides if the replica should vote for the given block.
func (hs *SimpleHotStuff) VoteRule(proposal hotstuff.ProposeMsg) bool {
	block := proposal.Block

	// Rule 1: can only vote in increasing rounds
	if block.View() < hs.comps.Synchronizer.View() {
		hs.comps.Logger.Info("VoteRule: block view too low")
		return false
	}

	parent, ok := hs.comps.BlockChain.Get(block.QuorumCert().BlockHash())
	if !ok {
		hs.comps.Logger.Info("VoteRule: missing parent block: ", block.QuorumCert().BlockHash())
		return false
	}

	// Rule 2: can only vote if parent's view is greater than or equal to locked block's view.
	if parent.View() < hs.locked.View() {
		hs.comps.Logger.Info("OnPropose: parent too old")
		return false
	}

	return true
}

// CommitRule decides if an ancestor of the block can be committed, and returns the ancestor, otherwise returns nil.
func (hs *SimpleHotStuff) CommitRule(block *hotstuff.Block) *hotstuff.Block {
	// will consider if the great-grandparent of the new block can be committed.
	p, ok := hs.comps.BlockChain.Get(block.QuorumCert().BlockHash())
	if !ok {
		return nil
	}

	gp, ok := hs.comps.BlockChain.Get(p.QuorumCert().BlockHash())
	if ok && gp.View() > hs.locked.View() {
		hs.locked = gp
		hs.comps.Logger.Debug("Locked: ", gp)
	} else if !ok {
		return nil
	}

	ggp, ok := hs.comps.BlockChain.Get(gp.QuorumCert().BlockHash())
	// we commit the great-grandparent of the block if its grandchild is certified,
	// which we already know is true because the new block contains the grandchild's certificate,
	// and if the great-grandparent's view + 2 equals the grandchild's view.
	if ok && ggp.View()+2 == p.View() {
		return ggp
	}
	return nil
}

// ChainLength returns the number of blocks that need to be chained together in order to commit.
func (hs *SimpleHotStuff) ChainLength() int {
	return 3
}
