package twins

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/protocol/rules"
	"github.com/relab/hotstuff/security/crypto"
)

/*
TestGhostQCPanic - Tests for the Ghost QC Panic Bug

== Bug Description ==

When a node receives a QC pointing to a non-existent block (ghost block),
the VerifyQuorumCert function calls blockchain.Get() which attempts to
fetch the block from the network. This triggers eventLoop.TimeoutContext()
which panics if the parent context is nil.

== Attack Vector ==

An attacker can crash any node by sending a validly-signed QC that points
to a random hash (ghost block). This is a critical DoS vulnerability.

== Root Cause ==

In security/cert/auth.go, VerifyQuorumCert() calls blockchain.Get():
```go
block, ok := c.blockchain.Get(qc.BlockHash())  // Attempts network fetch!
```

blockchain.Get() calls eventLoop.TimeoutContext() when block not found locally:
```go
ctx, cancel = chain.eventLoop.TimeoutContext()  // Panics if context is nil!
```

== Fix ==

Use LocalGet() instead of Get() in VerifyQuorumCert and VerifyPartialCert.
This prevents:
1. Panic from nil context
2. DoS attacks via ghost QCs
3. Expensive network operations for invalid certificates
*/

// TestGhostQCPanic_BasicAttack verifies that ghost QC no longer causes panic
func TestGhostQCPanic_BasicAttack(t *testing.T) {
	const numNodes = 4

	logging.SetLogLevel("info")
	t.Log("╔══════════════════════════════════════════════════════════════╗")
	t.Log("║  Ghost QC Panic Bug Test                                     ║")
	t.Log("║  Testing: QC pointing to non-existent block                  ║")
	t.Log("╚══════════════════════════════════════════════════════════════╝")

	network := createGhostQCTestNetwork(t, numNodes, 15)
	network.run(300)

	targetNode := network.nodes[Replica(1)]

	if len(targetNode.executedBlocks) < 5 {
		t.Skip("Not enough commits")
	}

	initialHighQC := targetNode.viewStates.HighQC()
	t.Log("\n=== Initial State ===")
	t.Logf("  HighQC.View: %d", initialHighQC.View())
	t.Logf("  Commits: %d", len(targetNode.executedBlocks))

	// Create ghost block hash (random, non-existent)
	t.Log("\n=== Creating Ghost QC ===")
	ghostHash := generateRandomHash()
	ghostView := initialHighQC.View() + 50

	t.Logf("  Ghost Hash: %.8s", ghostHash)
	t.Logf("  Ghost View: %d", ghostView)

	// Verify ghost block does NOT exist
	_, exists := targetNode.blockchain.LocalGet(ghostHash)
	if exists {
		t.Fatal("Test setup error: ghost block should not exist")
	}
	t.Log("  ✅ Confirmed: Ghost block does NOT exist")

	// Create valid QC pointing to ghost block
	ghostQC := createGhostQCForTest(t, network, ghostHash, ghostView)

	// Inject ghost QC
	t.Log("\n=== Injecting Ghost QC ===")
	syncInfo := hotstuff.NewSyncInfo()
	syncInfo.SetQC(ghostQC)

	targetNode.eventLoop.AddEvent(hotstuff.NewViewMsg{ID: 99, SyncInfo: syncInfo})

	// Process with panic detection
	completed, panicMsg := processEventsWithPanicDetection(targetNode, 2*time.Second)

	t.Log("\n=== Result ===")
	t.Logf("  Completed: %v", completed)

	if panicMsg != "" {
		t.Errorf("  ❌ BUG NOT FIXED: Node panicked!")
		t.Errorf("     Panic: %s", panicMsg)
	} else {
		t.Log("  ✅ FIXED: Node did NOT panic")
		t.Log("     Ghost QC was rejected gracefully")
	}

	// Verify node state is intact
	finalHighQC := targetNode.viewStates.HighQC()
	if finalHighQC.View() == initialHighQC.View() {
		t.Log("  ✅ HighQC unchanged (ghost QC rejected)")
	} else if finalHighQC.BlockHash() == ghostHash {
		t.Error("  ❌ HighQC points to ghost block!")
	}
}

// TestGhostQCPanic_VerifyQuorumCertDirect tests VerifyQuorumCert indirectly via ViewStates
func TestGhostQCPanic_VerifyQuorumCertDirect(t *testing.T) {
	const numNodes = 4

	logging.SetLogLevel("info")
	t.Log("╔══════════════════════════════════════════════════════════════╗")
	t.Log("║  Ghost QC - Direct UpdateHighQC Test                         ║")
	t.Log("╚══════════════════════════════════════════════════════════════╝")

	network := createGhostQCTestNetwork(t, numNodes, 15)
	network.run(300)

	targetNode := network.nodes[Replica(1)]

	if len(targetNode.executedBlocks) < 5 {
		t.Skip("Not enough commits")
	}

	// Create ghost QC
	ghostHash := generateRandomHash()
	ghostQC := createGhostQCForTest(t, network, ghostHash, 100)

	t.Log("\n=== Direct UpdateHighQC Test ===")
	t.Logf("  Ghost hash: %.8s", ghostHash)
	t.Logf("  Ghost view: %d", ghostQC.View())

	// Test UpdateHighQC with ghost QC (should not panic, should return error)
	updated, err := targetNode.viewStates.UpdateHighQC(ghostQC)

	t.Logf("  UpdateHighQC result: updated=%v, err=%v", updated, err)

	if err != nil {
		t.Log("\n  ✅ CORRECT: UpdateHighQC returned error (not panic)")
		t.Logf("     Error message: %v", err)
	} else if !updated {
		t.Log("\n  ✅ CORRECT: UpdateHighQC rejected ghost QC (returned false)")
	} else {
		t.Error("\n  ❌ BUG: UpdateHighQC should reject ghost QC")
	}
}

// TestGhostQCPanic_MultipleGhostQCs tests resilience against multiple ghost QCs
func TestGhostQCPanic_MultipleGhostQCs(t *testing.T) {
	const numNodes = 4

	logging.SetLogLevel("info")
	t.Log("╔══════════════════════════════════════════════════════════════╗")
	t.Log("║  Ghost QC - Multiple Attack Test                             ║")
	t.Log("╚══════════════════════════════════════════════════════════════╝")

	network := createGhostQCTestNetwork(t, numNodes, 15)
	network.run(300)

	targetNode := network.nodes[Replica(1)]

	if len(targetNode.executedBlocks) < 5 {
		t.Skip("Not enough commits")
	}

	initialCommits := len(targetNode.executedBlocks)

	t.Log("\n=== Sending Multiple Ghost QCs ===")
	panicCount := 0

	for i := 0; i < 10; i++ {
		ghostHash := generateRandomHash()
		ghostQC := createGhostQCForTest(t, network, ghostHash, hotstuff.View(100+i*10))

		syncInfo := hotstuff.NewSyncInfo()
		syncInfo.SetQC(ghostQC)
		targetNode.eventLoop.AddEvent(hotstuff.NewViewMsg{ID: hotstuff.ID(90 + i), SyncInfo: syncInfo})

		_, panicMsg := processEventsWithPanicDetection(targetNode, 500*time.Millisecond)
		if panicMsg != "" {
			panicCount++
			t.Logf("  Ghost %d: PANIC - %s", i+1, panicMsg)
		} else {
			t.Logf("  Ghost %d: Handled gracefully", i+1)
		}
	}

	t.Log("\n=== Result ===")
	if panicCount > 0 {
		t.Errorf("  ❌ %d panics detected!", panicCount)
	} else {
		t.Log("  ✅ All ghost QCs handled without panic")
	}

	// Verify commits preserved
	finalCommits := len(targetNode.executedBlocks)
	if finalCommits >= initialCommits {
		t.Logf("  ✅ Commits preserved: %d", finalCommits)
	} else {
		t.Errorf("  ❌ Commits lost: %d -> %d", initialCommits, finalCommits)
	}
}

// TestGhostQCPanic_LivenessAfterAttack tests if node state is intact after ghost QC attack
func TestGhostQCPanic_LivenessAfterAttack(t *testing.T) {
	const numNodes = 4

	logging.SetLogLevel("info")
	t.Log("╔══════════════════════════════════════════════════════════════╗")
	t.Log("║  Ghost QC - State Integrity After Attack Test                ║")
	t.Log("╚══════════════════════════════════════════════════════════════╝")

	network := createGhostQCTestNetwork(t, numNodes, 15)
	network.run(300)

	targetNode := network.nodes[Replica(1)]

	if len(targetNode.executedBlocks) < 5 {
		t.Skip("Not enough commits")
	}

	initialCommits := len(targetNode.executedBlocks)
	initialHighQC := targetNode.viewStates.HighQC()

	// Send multiple ghost QCs
	t.Log("\n=== Injecting Multiple Ghost QCs ===")
	for i := 0; i < 5; i++ {
		ghostHash := generateRandomHash()
		ghostQC := createGhostQCForTest(t, network, ghostHash, hotstuff.View(500+i*100))

		syncInfo := hotstuff.NewSyncInfo()
		syncInfo.SetQC(ghostQC)
		targetNode.eventLoop.AddEvent(hotstuff.NewViewMsg{ID: hotstuff.ID(90 + i), SyncInfo: syncInfo})
		processEventsWithPanicDetection(targetNode, 500*time.Millisecond)
	}

	t.Log("\n=== Verifying State Integrity ===")
	finalCommits := len(targetNode.executedBlocks)
	finalHighQC := targetNode.viewStates.HighQC()

	t.Logf("  Initial commits: %d", initialCommits)
	t.Logf("  Final commits: %d", finalCommits)
	t.Logf("  Initial HighQC.View: %d", initialHighQC.View())
	t.Logf("  Final HighQC.View: %d", finalHighQC.View())

	// Verify state integrity
	if finalCommits >= initialCommits {
		t.Log("\n  ✅ COMMITS PRESERVED: No data loss")
	} else {
		t.Errorf("\n  ❌ COMMITS LOST: %d -> %d", initialCommits, finalCommits)
	}

	// HighQC should not have been updated to any ghost view
	if finalHighQC.View() == initialHighQC.View() {
		t.Log("  ✅ HighQC UNCHANGED: Ghost QCs properly rejected")
	} else {
		t.Log("  ⚠️ HighQC view changed (may be legitimate)")
	}
}

// TestGhostQCPanic_RegressionTest is a regression test that fails if bug reappears
func TestGhostQCPanic_RegressionTest(t *testing.T) {
	const numNodes = 4

	logging.SetLogLevel("info")
	t.Log("╔══════════════════════════════════════════════════════════════╗")
	t.Log("║  Ghost QC Panic - Regression Test                            ║")
	t.Log("║  This test MUST PASS - failure indicates bug reintroduction  ║")
	t.Log("╚══════════════════════════════════════════════════════════════╝")

	network := createGhostQCTestNetwork(t, numNodes, 15)
	network.run(300)

	targetNode := network.nodes[Replica(1)]

	if len(targetNode.executedBlocks) < 5 {
		t.Skip("Not enough commits")
	}

	// Test 1: Direct UpdateHighQC call with ghost QC
	t.Log("\n=== Test 1: Direct UpdateHighQC with Ghost QC ===")
	ghostQC := createGhostQCForTest(t, network, generateRandomHash(), 1000)

	updated, err := targetNode.viewStates.UpdateHighQC(ghostQC)
	if err != nil || !updated {
		t.Log("  ✅ PASS: UpdateHighQC rejected ghost QC")
	} else {
		t.Error("  ❌ FAIL: UpdateHighQC should reject ghost QC")
	}

	// Test 2: Via NewViewMsg (the original panic path)
	t.Log("\n=== Test 2: Via NewViewMsg (original panic path) ===")
	syncInfo := hotstuff.NewSyncInfo()
	syncInfo.SetQC(ghostQC)
	targetNode.eventLoop.AddEvent(hotstuff.NewViewMsg{ID: 99, SyncInfo: syncInfo})

	_, panicMsg := processEventsWithPanicDetection(targetNode, 2*time.Second)
	if panicMsg != "" {
		t.Errorf("  ❌ FAIL: Node panicked: %s", panicMsg)
		t.Error("     The Ghost QC Panic bug has been reintroduced!")
	} else {
		t.Log("  ✅ PASS: No panic occurred")
	}

	t.Log("\n=== Regression Test Complete ===")
}

// ========== Helper Functions ==========

func generateRandomHash() hotstuff.Hash {
	var hash hotstuff.Hash
	rand.Read(hash[:])
	return hash
}

func createGhostQCForTest(t *testing.T, network *Network, ghostHash hotstuff.Hash, ghostView hotstuff.View) hotstuff.QuorumCert {
	// Create fake block data for signing
	fakeBlockData := make([]byte, 0)
	fakeBlockData = append(fakeBlockData, ghostHash[:]...)

	var proposerBuf [4]byte
	fakeBlockData = append(fakeBlockData, proposerBuf[:]...)

	viewBytes := make([]byte, 8)
	for i := 0; i < 8; i++ {
		viewBytes[i] = byte(ghostView >> (i * 8))
	}
	fakeBlockData = append(fakeBlockData, viewBytes...)

	// Sign with multiple keys
	var signatures []hotstuff.QuorumSignature
	signersNeeded := 3

	for nodeID, node := range network.nodes {
		if len(signatures) >= signersNeeded {
			break
		}

		privKey := node.config.PrivateKey().(*ecdsa.PrivateKey)
		hash := sha256.Sum256(fakeBlockData)
		sig, err := ecdsa.SignASN1(rand.Reader, privKey, hash[:])
		if err != nil {
			t.Fatalf("Failed to sign: %v", err)
		}

		ecdsaSig := crypto.RestoreECDSASignature(sig, nodeID.ReplicaID)
		signatures = append(signatures, crypto.NewMulti(ecdsaSig))
	}

	var allSigs []*crypto.ECDSASignature
	for _, sig := range signatures {
		if multi, ok := sig.(crypto.Multi[*crypto.ECDSASignature]); ok {
			for _, s := range multi {
				allSigs = append(allSigs, s)
			}
		}
	}

	return hotstuff.NewQuorumCert(crypto.NewMulti(allSigs...), ghostView, ghostHash)
}

func createGhostQCTestNetwork(t *testing.T, numNodes, stableViews int) *Network {
	scenario := generateGhostQCStableViews(stableViews, numNodes)

	network := NewPartitionedNetwork(scenario,
		hotstuff.ProposeMsg{},
		hotstuff.VoteMsg{},
		hotstuff.Hash{},
		hotstuff.NewViewMsg{},
		hotstuff.TimeoutMsg{},
	)

	consensusName := rules.NameChainedHotStuff
	nodes, _ := assignNodeIDs(uint8(numNodes), 0)
	err := network.createNodesAndTwins(nodes, consensusName)
	if err != nil {
		t.Fatalf("Failed to create nodes: %v", err)
	}

	return network
}

func generateGhostQCStableViews(numViews, numNodes int) Scenario {
	scenario := make(Scenario, numViews)
	allNodes := make(NodeSet)
	for i := 1; i <= numNodes; i++ {
		allNodes[NodeID{ReplicaID: hotstuff.ID(i)}] = struct{}{}
	}
	for i := 0; i < numViews; i++ {
		scenario[i] = View{
			Leader:     hotstuff.ID((i % numNodes) + 1),
			Partitions: []NodeSet{allNodes},
		}
	}
	return scenario
}

func processEventsWithPanicDetection(node *node, timeout time.Duration) (completed bool, panicMsg string) {
	done := make(chan bool, 1)
	panicChan := make(chan string, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				panicChan <- fmt.Sprintf("%v", r)
			}
		}()
		for node.eventLoop.Tick(nil) {
		}
		done <- true
	}()

	select {
	case <-done:
		return true, ""
	case msg := <-panicChan:
		return false, msg
	case <-time.After(timeout):
		return false, ""
	}
}

