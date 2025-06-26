package protocol_test

import (
	"bytes"
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/protocol"
	"github.com/relab/hotstuff/security/crypto"
)

func TestNextView(t *testing.T) {
	essentials := testutil.WireUpEssentials(t, 1, crypto.NameECDSA)
	states, err := protocol.NewViewStates(essentials.BlockChain(), essentials.Authority())
	if err != nil {
		t.Fatal(err)
	}
	newView := states.NextView()
	if newView != hotstuff.View(2) {
		t.Fail()
	}
}

func TestUpdateCerts(t *testing.T) {
	set := testutil.NewEssentialsSet(t, 4, crypto.NameECDSA)
	subject := set[0]
	states, err := protocol.NewViewStates(subject.BlockChain(), subject.Authority())
	if err != nil {
		t.Fatal(err)
	}
	block := hotstuff.NewBlock(
		hotstuff.GetGenesis().Hash(),
		hotstuff.GetGenesis().QuorumCert(),
		&clientpb.Batch{},
		1,
		1,
	)
	subject.BlockChain().Store(block)
	signers := set.Signers()

	// need only 3 for a quorum
	qc := testutil.CreateQC(t, block, signers...)

	if err := states.UpdateHighQC(qc); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(qc.ToBytes(), states.HighQC().ToBytes()) {
		t.Fatal("quorum cert was not updated")
	}

	tc := testutil.CreateTC(t, 1, signers)
	states.UpdateHighTC(tc)
	if !bytes.Equal(tc.ToBytes(), states.HighTC().ToBytes()) {
		t.Fatal("timeout cert was not updated")
	}
}

func TestUpdateCommit(t *testing.T) {
	block := hotstuff.NewBlock(
		hotstuff.GetGenesis().Hash(),
		hotstuff.GetGenesis().QuorumCert(),
		&clientpb.Batch{},
		1,
		1,
	)
	essentials := testutil.WireUpEssentials(t, 1, crypto.NameECDSA)
	states, err := protocol.NewViewStates(essentials.BlockChain(), essentials.Authority())
	if err != nil {
		t.Fatal(err)
	}
	states.UpdateCommittedBlock(block)
	if block != states.CommittedBlock() {
		t.Fatal("committed block was not updated")
	}
}
