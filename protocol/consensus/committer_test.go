package consensus_test

import (
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/protocol"
	"github.com/relab/hotstuff/protocol/consensus"
	"github.com/relab/hotstuff/protocol/rules"
	"github.com/relab/hotstuff/security/crypto"
)

func wireUpCommitter(
	t *testing.T,
	essentials *testutil.Essentials,
	viewStates *protocol.ViewStates,
	commitRuler consensus.CommitRuler,
) *consensus.Committer {
	t.Helper()
	return consensus.NewCommitter(
		essentials.EventLoop(),
		essentials.Logger(),
		essentials.Blockchain(),
		viewStates,
		commitRuler,
	)
}

func TestValidCommit(t *testing.T) {
	essentials := testutil.WireUpEssentials(t, 1, crypto.NameECDSA)
	viewStates, err := protocol.NewViewStates(
		essentials.Blockchain(),
		essentials.Authority(),
	)
	if err != nil {
		t.Fatal(err)
	}
	// create a valid chain of blocks
	chain := essentials.Blockchain()
	parent := hotstuff.GetGenesis()
	var firstBlock *hotstuff.Block = nil
	chs := rules.NewChainedHotStuff(
		essentials.Logger(),
		essentials.RuntimeCfg(),
		essentials.Blockchain(),
	)
	for range chs.ChainLength() {
		block := testutil.CreateParentedBlock(t, 1, parent)
		if firstBlock == nil {
			firstBlock = block
		}
		chain.Store(block)
		parent = block
	}
	blockToCommit := testutil.CreateParentedBlock(t, 1, parent)
	committer := wireUpCommitter(t, essentials, viewStates, chs)
	if err := committer.TryCommit(blockToCommit); err != nil {
		t.Fatal(err)
	}
	if firstBlock != viewStates.CommittedBlock() {
		t.Fatal("incorrect block was committed")
	}
}
