package protocol_test

import (
	"bytes"
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/protocol"
	"github.com/relab/hotstuff/security/cert"
	"github.com/relab/hotstuff/security/crypto/ecdsa"
)

func TestUpdateView(t *testing.T) {
	essentials := testutil.WireUpEssentials(t, 1, ecdsa.ModuleName)
	states, err := protocol.NewViewStates(essentials.BlockChain(), essentials.Authority())
	if err != nil {
		t.Fatal(err)
	}
	view := hotstuff.View(5)
	states.UpdateView(view)
	if view != states.View() {
		t.Fail()
	}
}

func TestUpdateCerts(t *testing.T) {
	essentials := testutil.WireUpEssentials(t, 1, ecdsa.ModuleName)
	states, err := protocol.NewViewStates(essentials.BlockChain(), essentials.Authority())
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
	essentials.BlockChain().Store(block)
	signers := make([]*cert.Authority, 0)
	signers = append(signers, essentials.Authority())
	for i := range 3 {
		id := hotstuff.ID(i + 2)
		replica := testutil.WireUpEssentials(t, id, ecdsa.ModuleName)
		essentials.RuntimeCfg().AddReplica(&hotstuff.ReplicaInfo{
			ID:     id,
			PubKey: replica.RuntimeCfg().PrivateKey().Public(),
		})
		signers = append(signers, replica.Authority())
	}

	// need only 3 for a quorum
	qc := testutil.CreateQC(t, block, signers)

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
	essentials := testutil.WireUpEssentials(t, 1, ecdsa.ModuleName)
	states, err := protocol.NewViewStates(essentials.BlockChain(), essentials.Authority())
	if err != nil {
		t.Fatal(err)
	}
	states.UpdateCommittedBlock(block)
	if block != states.CommittedBlock() {
		t.Fatal("committed block was not updated")
	}
}
