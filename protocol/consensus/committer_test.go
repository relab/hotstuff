package consensus_test

import (
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/rules/chainedhotstuff"
)

func wireUpCommitter(
	t *testing.T,
	essentials *testutil.Essentials,
	viewStates *protocol.ViewStates,
	commitRuler modules.CommitRuler,
) *consensus.Committer {
	t.Helper()
	return consensus.NewCommitter(
		essentials.EventLoop(),
		essentials.Logger(),
		essentials.BlockChain(),
		viewStates,
		commitRuler,
	)
}

func TestValidCommit(t *testing.T) {
	essentials := testutil.WireUpEssentials(t, 1)
	viewStates, err := protocol.NewViewStates(
		essentials.BlockChain(),
		essentials.Authority(),
	)
	if err != nil {
		t.Fatal(err)
	}
	// create a valid chain of blocks
	chain := essentials.BlockChain()
	parent := hotstuff.GetGenesis()
	var firstBlock *hotstuff.Block = nil
	chs := chainedhotstuff.New(
		essentials.Logger(),
		essentials.BlockChain(),
	)
	for range chs.ChainLength() {
		block := testutil.CreateValidBlock(t, 1, parent)
		if firstBlock == nil {
			firstBlock = block
		}
		chain.Store(block)
		parent = block
	}
	blockToCommit := testutil.CreateValidBlock(t, 1, parent)
	committer := wireUpCommitter(t, essentials, viewStates, chs)
	if err := committer.TryCommit(blockToCommit); err != nil {
		t.Fatal(err)
	}
	if firstBlock != viewStates.CommittedBlock() {
		t.Fatal("incorrect block was committed")
	}
}
