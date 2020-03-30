package hotstuff

import (
	"bytes"
	"testing"
	"time"
)

type stubBackend struct{}

func (d *stubBackend) Init(hs *HotStuff)                                         {}
func (d *stubBackend) DoPropose(node *Node, qc *QuorumCert) (*QuorumCert, error) { return nil, nil }
func (d *stubBackend) DoNewView(leader ReplicaID, qc *QuorumCert) error          { return nil }
func (d *stubBackend) Close()                                                    {}

func TestSafeNode(t *testing.T) {
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
}

func TestUpdateQCHigh(t *testing.T) {
	key, _ := GeneratePrivateKey()
	hs := New(1, key, NewConfig(), &stubBackend{}, 10*time.Millisecond, nil)
	node1 := CreateLeaf(hs.genesis, []Command{Command("command1")}, hs.qcHigh, hs.genesis.Height+1)
	hs.nodes.Put(node1)
	qc1 := CreateQuorumCert(node1)

	if hs.UpdateQCHigh(qc1) {
		if hs.bLeaf.Hash() != node1.Hash() {
			t.Error("UpdateQCHigh failed to update the leaf node")
		}
		if !bytes.Equal(hs.qcHigh.toBytes(), qc1.toBytes()) {
			t.Error("UpdateQCHigh failed to update qcHigh")
		}

	} else {
		t.Error("UpdateQCHigh failed to complete")
	}

	node2 := CreateLeaf(node1, []Command{Command("command2")}, qc1, node1.Height+1)
	qc2 := CreateQuorumCert(node2)
	hs.UpdateQCHigh(qc2)

	if hs.UpdateQCHigh(qc1) {
		t.Error("UpdateQCHigh updated with outdated state given as input.")
	}

}

func TestUpdate(t *testing.T) {
	exec := make(chan []Command, 1)
	key, _ := GeneratePrivateKey()
	hs := New(1, key, NewConfig(), &stubBackend{}, 10*time.Millisecond, func(b []Command) { exec <- b })
	hs.QuorumSize = 0 // this accepts all QCs

	n1 := CreateLeaf(hs.genesis, []Command{Command("n1")}, hs.qcHigh, hs.genesis.Height+1)
	hs.nodes.Put(n1)
	n2 := CreateLeaf(n1, []Command{Command("n2")}, CreateQuorumCert(n1), n1.Height+1)
	hs.nodes.Put(n2)
	n3 := CreateLeaf(n2, []Command{Command("n3")}, CreateQuorumCert(n2), n2.Height+1)
	hs.nodes.Put(n3)
	n4 := CreateLeaf(n3, []Command{Command("n4")}, CreateQuorumCert(n3), n3.Height+1)
	hs.nodes.Put(n4)

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
	case b := <-exec:
		if !bytes.Equal(b[0], n1.Commands[0]) {
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
	hs := New(1, key, NewConfig(), &stubBackend{}, 10*time.Millisecond, nil)
	node1 := CreateLeaf(hs.genesis, []Command{Command("command1")}, hs.qcHigh, hs.genesis.Height+1)
	qc := CreateQuorumCert(node1)

	pc, err := hs.OnReceiveProposal(node1)

	if err != nil {
		t.Errorf("onReciveProposal failed with error: %w", err)
	}

	if pc == nil {
		t.Error("onReciveProposal failed to complete")
	} else {
		if _, ok := hs.nodes.Get(node1.Hash()); !ok {
			t.Error("onReciveProposal failed to place the new node in NodeStorage")
		}
		if hs.vHeight != node1.Height {
			t.Error("onReciveProposal failed to update the heigt of the replica")
		}
	}

	node2 := CreateLeaf(node1, []Command{Command("command2")}, qc, node1.Height+1)

	hs.OnReceiveProposal(node2)
	pc, err = hs.OnReceiveProposal(node1)

	if err == nil {
		t.Error("Node got accepted, expected rejection.")
	}
	if pc != nil {
		t.Errorf("Expected nil got: %v", pc)
	}
}

func TestExpectNodeFor(t *testing.T) {
	key, _ := GeneratePrivateKey()
	hs := New(1, key, NewConfig(), &stubBackend{}, time.Second, nil)
	node := CreateLeaf(hs.genesis, []Command{Command("test")}, hs.qcHigh, 1)
	qc := CreateQuorumCert(node)

	go func() {
		time.Sleep(100 * time.Millisecond)
		hs.OnReceiveProposal(node)
	}()

	n, ok := hs.expectNodeFor(qc)
	if !ok && n == nil {
		t.Fail()
	}
}
