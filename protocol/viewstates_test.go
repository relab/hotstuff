package protocol_test

import (
	"bytes"
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/internal/testutil"
	"github.com/relab/hotstuff/protocol"
	"github.com/relab/hotstuff/security/cert"
	"github.com/relab/hotstuff/security/crypto/ecdsa"
	"github.com/relab/hotstuff/wiring"
)

func wireUpViewStates(t *testing.T) (*protocol.ViewStates, *wiring.Core, *wiring.Security) {
	depsCore := wiring.NewCore(1, "test", testutil.GenerateECDSAKey(t))
	depsSecurity, err := wiring.NewSecurity(
		depsCore.EventLoop(),
		depsCore.Logger(),
		depsCore.RuntimeCfg(),
		testutil.NewMockSender(1),
		ecdsa.ModuleName,
	)
	if err != nil {
		t.Fatal(err)
	}
	states, err := protocol.NewViewStates(
		depsSecurity.BlockChain(),
		depsSecurity.Authority(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return states, depsCore, depsSecurity
}

func wireUpSigners(t *testing.T, parentCfg *core.RuntimeConfig, n uint) []*cert.Authority {
	signers := make([]*cert.Authority, 0)
	for i := range n {
		id := hotstuff.ID(i + 2)
		pk := testutil.GenerateECDSAKey(t)
		parentCfg.AddReplica(&hotstuff.ReplicaInfo{
			ID:     id,
			PubKey: pk.Public(),
		})
		depsCore := wiring.NewCore(id, "test", pk)
		depsSecurity, err := wiring.NewSecurity(
			depsCore.EventLoop(),
			depsCore.Logger(),
			depsCore.RuntimeCfg(),
			testutil.NewMockSender(id),
			ecdsa.ModuleName,
		)
		if err != nil {
			t.Fatal(err)
		}
		signers = append(signers, depsSecurity.Authority())
	}
	return signers
}

func TestUpdateView(t *testing.T) {
	states, _, _ := wireUpViewStates(t)
	view := hotstuff.View(5)
	states.UpdateView(view)
	if view != states.View() {
		t.Fail()
	}
}

func TestUpdateCerts(t *testing.T) {
	states, core, security := wireUpViewStates(t)
	block := hotstuff.NewBlock(
		hotstuff.GetGenesis().Hash(),
		hotstuff.GetGenesis().QuorumCert(),
		&clientpb.Batch{},
		1,
		1,
	)
	security.BlockChain().Store(block)
	signers := wireUpSigners(t, core.RuntimeCfg(), 3) // need only 3 for a quorum
	qc := testutil.CreateQC(t, block, signers)
	err := states.UpdateHighQC(qc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(qc.ToBytes(), states.HighQC().ToBytes()) {
		t.Fatal("quorum cert was not updated")
	}

	tc := testutil.CreateTC(t, 1, security.Authority(), signers)
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
	states, _, _ := wireUpViewStates(t)
	states.UpdateCommittedBlock(block)
	if block != states.CommittedBlock() {
		t.Fatal("committed block was not updated")
	}
}
