package hotstuff

import (
	"bytes"
	"crypto/ecdsa"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestSafeNode(t *testing.T) {
	key, _ := GeneratePrivateKey()
	hs := New(1, key, NewConfig(), nil, time.Second, nil, nil)

	n1 := createLeaf(hs.genesis, []byte("n1"), hs.qcHigh, hs.genesis.Height+1)
	hs.nodes.Put(n1)
	n2 := createLeaf(n1, []byte("n2"), CreateQuorumCert(n1), n1.Height+1)
	hs.nodes.Put(n2)

	if !hs.safeNode(n2) {
		t.Error("SafeNode rejected node, but both rules should have passed it.")
	}

	hs.bLock = n2

	n3 := createLeaf(n1, []byte("n3"), CreateQuorumCert(n1), n2.Height+1)
	hs.nodes.Put(n3)
	n4 := createLeaf(n3, []byte("n4"), CreateQuorumCert(n3), n3.Height+1)
	hs.nodes.Put(n4)

	if !hs.safeNode(n4) {
		t.Error("SafeNode rejected node, but liveness rule should have passed it.")
	}

	n5 := createLeaf(n2, []byte("n5"), CreateQuorumCert(n2), n2.Height+1)
	hs.nodes.Put(n5)
	n6 := createLeaf(n5, []byte("n6"), CreateQuorumCert(n5), n5.Height+1)
	hs.nodes.Put(n6)
	// intentionally violates liveness rule
	n7 := createLeaf(n6, []byte("n7"), CreateQuorumCert(n6), 1)
	hs.nodes.Put(n7)

	if !hs.safeNode(n7) {
		t.Error("SafeNode rejected node, but safety rule should have passed it.")
	}

	bad := createLeaf(hs.genesis, []byte("bad"), CreateQuorumCert(hs.genesis), hs.genesis.Height+1)
	hs.nodes.Put(bad)

	if hs.safeNode(bad) {
		t.Error("SafeNode accepted node, but none of the rules should have passed it.")
	}
}

func TestUpdateQCHigh(t *testing.T) {
	key, _ := GeneratePrivateKey()
	hs := New(1, key, NewConfig(), nil, time.Second, nil, nil)
	node1 := createLeaf(hs.genesis, []byte("command1"), hs.qcHigh, hs.genesis.Height+1)
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

	node2 := createLeaf(node1, []byte("command2"), qc1, node1.Height+1)
	qc2 := CreateQuorumCert(node2)
	hs.UpdateQCHigh(qc2)

	if hs.UpdateQCHigh(qc1) {
		t.Error("UpdateQCHigh updated with outdated state given as input.")
	}

}

func TestUpdate(t *testing.T) {
	exec := make(chan []byte, 1)
	key, _ := GeneratePrivateKey()
	hs := New(1, key, NewConfig(), nil, time.Second, nil, func(b []byte) { exec <- b })
	hs.QuorumSize = 0 // this accepts all QCs

	n1 := createLeaf(hs.genesis, []byte("n1"), hs.qcHigh, hs.genesis.Height+1)
	hs.nodes.Put(n1)
	n2 := createLeaf(n1, []byte("n2"), CreateQuorumCert(n1), n1.Height+1)
	hs.nodes.Put(n2)
	n3 := createLeaf(n2, []byte("n3"), CreateQuorumCert(n2), n2.Height+1)
	hs.nodes.Put(n3)
	n4 := createLeaf(n3, []byte("n4"), CreateQuorumCert(n3), n3.Height+1)
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
		if !bytes.Equal(b, n1.Command) {
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
	hs := New(1, key, NewConfig(), nil, time.Second, nil, nil)
	node1 := createLeaf(hs.genesis, []byte("command1"), hs.qcHigh, hs.genesis.Height+1)
	qc := CreateQuorumCert(node1)

	pc, err := hs.onReceiveProposal(node1)

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

	node2 := createLeaf(node1, []byte("command2"), qc, node1.Height+1)

	hs.onReceiveProposal(node2)
	pc, err = hs.onReceiveProposal(node1)

	if err == nil {
		t.Error("Node got accepted, expected rejection.")
	}
	if pc != nil {
		t.Errorf("Expected nil got: %v", pc)
	}

}

// This test verifies that the entire stack works.
func TestHotStuff(t *testing.T) {
	keys := make(map[ReplicaID]*ecdsa.PrivateKey)
	keys[1], _ = GeneratePrivateKey()
	keys[2], _ = GeneratePrivateKey()
	keys[3], _ = GeneratePrivateKey()
	keys[4], _ = GeneratePrivateKey()

	config := NewConfig()
	config.Replicas[1] = &ReplicaInfo{ID: 1, Socket: "127.0.0.1:13371", PubKey: &keys[1].PublicKey}
	config.Replicas[2] = &ReplicaInfo{ID: 2, Socket: "127.0.0.1:13372", PubKey: &keys[2].PublicKey}
	config.Replicas[3] = &ReplicaInfo{ID: 3, Socket: "127.0.0.1:13373", PubKey: &keys[3].PublicKey}
	config.Replicas[4] = &ReplicaInfo{ID: 4, Socket: "127.0.0.1:13374", PubKey: &keys[4].PublicKey}

	out := make(map[ReplicaID]chan []byte)
	out[1] = make(chan []byte, 1)
	out[2] = make(chan []byte, 2)
	out[3] = make(chan []byte, 3)
	out[4] = make(chan []byte, 4)

	// wont give pm a HotStuff instance, because we only need GetLeader()
	pm := &FixedLeaderPacemaker{Leader: 1}
	commands := make(chan []byte, 1)

	replicas := make(map[ReplicaID]*HotStuff)
	replicas[1] = New(1, keys[1], config, pm, time.Second, commands, func(b []byte) { out[1] <- b })
	replicas[2] = New(2, keys[2], config, pm, time.Second, nil, func(b []byte) { out[2] <- b })
	replicas[3] = New(3, keys[3], config, pm, time.Second, nil, func(b []byte) { out[3] <- b })
	replicas[4] = New(4, keys[4], config, pm, time.Second, nil, func(b []byte) { out[4] <- b })

	var wg sync.WaitGroup
	wg.Add(len(replicas))
	for id := range replicas {
		go func(id ReplicaID) {
			err := replicas[id].Init(fmt.Sprintf("1337%d", id))
			if err != nil {
				t.Errorf("Failed to init replica %d: %v", id, err)
			}
			wg.Done()
		}(id)
	}

	wg.Wait()

	test := [][]byte{[]byte("DECIDE"), []byte("COMMIT"), []byte("PRECOMMIT"), []byte("PROPOSE")}
	for _, t := range test {
		commands <- t
		replicas[1].Propose()
	}

	for id, r := range replicas {
		r.Close()

		if !bytes.Equal(r.bExec.Command, test[0]) {
			t.Errorf("Replica %d: Incorrect bExec.Command: Got '%s', want '%s'", id, r.bExec.Command, test[0])
		}
		if !bytes.Equal(r.bLock.Command, test[1]) {
			t.Errorf("Replica %d: Incorrect bLock.Command: Got '%s', want '%s'", id, r.bLock.Command, test[1])
		}
		// leader will have progressed further due to UpdateQCHigh being called at the end of Propose()
		if r.id == pm.GetLeader() {
			if !bytes.Equal(r.bLeaf.Command, test[3]) {
				t.Errorf("Replica %d: Incorrect bLeaf.Command: Got '%s', want '%s'", id, r.bLeaf.Command, test[3])
			}
		} else if !bytes.Equal(r.bLeaf.Command, test[2]) {
			t.Errorf("Replica %d: Incorrect bLeaf.Command: Got '%s', want '%s'", id, r.bLeaf.Command, test[2])
		}
		fail := false
		select {
		case o := <-out[id]:
			fail = !bytes.Equal(o, test[0])
		default:
			fail = true
		}
		if fail {
			t.Errorf("Replica %d: Incorrect output!", id)
		}
	}
}
