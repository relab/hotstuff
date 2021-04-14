package consensus_test

import (
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus/chainedhotstuff"
	"github.com/relab/hotstuff/internal/mocks"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/synchronizer"
)

// TestVote checks that a leader can collect votes on a proposal to form a QC
func TestVote(t *testing.T) {
	const n = 4
	ctrl := gomock.NewController(t)

	chs := chainedhotstuff.New()
	bl := testutil.CreateBuilders(t, ctrl, n)
	synchronizer := synchronizer.New(testutil.FixedTimeout(time.Second))
	bl[0].Register(chs, synchronizer)
	hl := bl.Build()
	hs := hl[0]

	// expect that the replica will propose after receiving enough votes.
	hs.Manager().(*mocks.MockManager).EXPECT().Propose(gomock.AssignableToTypeOf(hotstuff.GetGenesis()))

	b := testutil.NewProposeMsg(hotstuff.GetGenesis().Hash(), hl[0].ViewSynchronizer().HighQC(), "test", 1, 1)

	chs.OnPropose(b)

	for i, signer := range hl.Signers()[1:] {
		pc, err := signer.CreatePartialCert(b.Block)
		if err != nil {
			t.Fatalf("Failed to create partial certificate: %v", err)
		}
		hs.VotingMachine().OnVote(hotstuff.VoteMsg{ID: hotstuff.ID(i + 1), PartialCert: pc})
	}

	if hl[0].ViewSynchronizer().HighQC().BlockHash() != b.Block.Hash() {
		t.Errorf("HighQC was not updated.")
	}
}
