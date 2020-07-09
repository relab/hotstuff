package consensus

import (
	"bytes"
	"testing"
	"time"

	. "github.com/relab/hotstuff/config"
	. "github.com/relab/hotstuff/data"
)

/* func TestSafeNode(t *testing.T) {
	key, _ := GeneratePrivateKey()
	hs := New(1, key, NewConfig(), &stubBackend{}, 10*time.Millisecond, nil)

	n1 := CreateLeaf(hs.genesis, []Command{Command("n1")}, hs.qcHigh, hs.genesis.Height+1)
	hs.nodes.Put(n1)
	n2 := CreateLeaf(n1, []Command{Command("n2")}, CreateQuorumCert(n1), n1.Height+1)
	hs.nodes.Put(n2)

	if !hs.safeNode(n2) {
		t.Error("SafeNode rejected node, but both rules should have passed it.")
	}

	hs.bLock = n2

	n3 := CreateLeaf(n1, []Command{Command("n3")}, CreateQuorumCert(n1), n2.Height+1)
	hs.nodes.Put(n3)
	n4 := CreateLeaf(n3, []Command{Command("n4")}, CreateQuorumCert(n3), n3.Height+1)
	hs.nodes.Put(n4)

	if !hs.safeNode(n4) {
		t.Error("SafeNode rejected node, but liveness rule should have passed it.")
	}

	n5 := CreateLeaf(n2, []Command{Command("n5")}, CreateQuorumCert(n2), n2.Height+1)
	hs.nodes.Put(n5)
	n6 := CreateLeaf(n5, []Command{Command("n6")}, CreateQuorumCert(n5), n5.Height+1)
	hs.nodes.Put(n6)
	// intentionally violates liveness rule
	n7 := CreateLeaf(n6, []Command{Command("n7")}, CreateQuorumCert(n6), 1)
	hs.nodes.Put(n7)

	if !hs.safeNode(n7) {
		t.Error("SafeNode rejected node, but safety rule should have passed it.")
	}

	bad := CreateLeaf(hs.genesis, []Command{Command("bad")}, CreateQuorumCert(hs.genesis), hs.genesis.Height+1)
	hs.nodes.Put(bad)

	if hs.safeNode(bad) {
		t.Error("SafeNode accepted node, but none of the rules should have passed it.")
	}
} */

func TestUpdateQCHigh(t *testing.T) {
	key, _ := GeneratePrivateKey()
	hs := New(NewConfig(1, key, nil))
	block1 := CreateLeaf(hs.genesis, []Command{Command("command1")}, hs.qcHigh, hs.genesis.Height+1)
	hs.Blocks.Put(block1)
	qc1 := CreateQuorumCert(block1)

	if hs.UpdateQCHigh(qc1) {
		if hs.bLeaf.Hash() != block1.Hash() {
			t.Error("UpdateQCHigh failed to update the leaf block")
		}
		if !bytes.Equal(hs.qcHigh.ToBytes(), qc1.ToBytes()) {
			t.Error("UpdateQCHigh failed to update qcHigh")
		}

	} else {
		t.Error("UpdateQCHigh failed to complete")
	}

	block2 := CreateLeaf(block1, []Command{Command("command2")}, qc1, block1.Height+1)
	hs.Blocks.Put(block2)
	qc2 := CreateQuorumCert(block2)
	hs.UpdateQCHigh(qc2)

	if hs.UpdateQCHigh(qc1) {
		t.Error("UpdateQCHigh updated with outdated state given as input.")
	}
}

func TestUpdate(t *testing.T) {
	key, _ := GeneratePrivateKey()
	hs := New(NewConfig(1, key, nil))
	hs.Config.QuorumSize = 0 // this accepts all QCs

	n1 := CreateLeaf(hs.genesis, []Command{Command("n1")}, hs.qcHigh, hs.genesis.Height+1)
	hs.Blocks.Put(n1)
	n2 := CreateLeaf(n1, []Command{Command("n2")}, CreateQuorumCert(n1), n1.Height+1)
	hs.Blocks.Put(n2)
	n3 := CreateLeaf(n2, []Command{Command("n3")}, CreateQuorumCert(n2), n2.Height+1)
	hs.Blocks.Put(n3)
	n4 := CreateLeaf(n3, []Command{Command("n4")}, CreateQuorumCert(n3), n3.Height+1)
	hs.Blocks.Put(n4)

	// PROPOSE on n1
	hs.update(n1)

	// PRECOMMIT on n1, PROPOSE on n2
	hs.update(n2)
	// check that QCHigh and bLeaf updated
	if hs.bLeaf != n1 || hs.qcHigh != n2.Justify {
		t.Error("PRECOMMIT failed")
	}

	// COMMIT on n1, PRECOMMIT on n2, PROPOSE on n3
	hs.update(n3)
	// check that bLock got updated
	if hs.bLock != n1 {
		t.Error("COMMIT failed")
	}

	// DECIDE on n1, COMMIT on n2, PRECOMIT on n3, PROPOSE on n4
	hs.update(n4)
	// check that bExec got updated and n1 got executed
	success := true
	if hs.bExec != n1 {
		success = false
	}

	select {
	case b := <-hs.GetExec():
		if b[0] != n1.Commands[0] {
			success = false
		}
	case <-time.After(time.Second):
		success = false
	}

	if !success {
		t.Error("DECIDE failed")
	}
}

func TestOnReciveProposal(t *testing.T) {
	key, _ := GeneratePrivateKey()
	hs := New(NewConfig(1, key, nil))
	block1 := CreateLeaf(hs.genesis, []Command{Command("command1")}, hs.qcHigh, hs.genesis.Height+1)
	qc := CreateQuorumCert(block1)

	pc, err := hs.OnReceiveProposal(block1)

	if err != nil {
		t.Errorf("onReciveProposal failed with error: %w", err)
	}

	if pc == nil {
		t.Error("onReciveProposal failed to complete")
	} else {
		if _, ok := hs.Blocks.Get(block1.Hash()); !ok {
			t.Error("onReciveProposal failed to place the new block in BlockStorage")
		}
		if hs.vHeight != block1.Height {
			t.Error("onReciveProposal failed to update the heigt of the replica")
		}
	}

	block2 := CreateLeaf(block1, []Command{Command("command2")}, qc, block1.Height+1)

	hs.OnReceiveProposal(block2)
	pc, err = hs.OnReceiveProposal(block1)

	if err == nil {
		t.Error("Block got accepted, expected rejection.")
	}
	if pc != nil {
		t.Errorf("Expected nil got: %v", pc)
	}
}

func TestExpectBlock(t *testing.T) {
	key, _ := GeneratePrivateKey()
	hs := New(NewConfig(1, key, nil))
	block := CreateLeaf(hs.genesis, []Command{Command("test")}, hs.qcHigh, 1)
	qc := CreateQuorumCert(block)

	go func() {
		time.Sleep(100 * time.Millisecond)
		hs.OnReceiveProposal(block)
	}()

	hs.mut.Lock()
	n, ok := hs.expectBlock(qc.BlockHash)
	hs.mut.Unlock()

	if !ok && n == nil {
		t.Fail()
	}
}
