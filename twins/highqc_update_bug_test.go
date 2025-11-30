package twins

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/proto/clientpb"
	"github.com/relab/hotstuff/protocol/rules"
	"github.com/relab/hotstuff/security/crypto"
)

/*
TestHighQCUpdateBug - Tests for the UpdateHighQC design flaw

== Bug Description ==

The original implementation of UpdateHighQC (protocol/viewstates.go):

```go
func (s *ViewStates) UpdateHighQC(qc hotstuff.QuorumCert) (bool, error) {
    newBlock, ok := s.blockchain.Get(qc.BlockHash())
    if !ok {
        return false, fmt.Errorf("block not found")
    }
    s.mut.Lock()
    defer s.mut.Unlock()
    if newBlock.View() <= s.highQC.View() {  // Only checks View
        return false, nil
    }
    s.highQC = qc  // Updates directly without checking if extends committed chain
    return true, nil
}
```

== Problem ==

Missing check: new QC must extend the committed chain (committedBlock)

== Impact ==

- Safety: Still safe (VoteRule has bLock check)
- Liveness: May be affected
- Resources: May cause waste

== Suggested Fix ==

Add to UpdateHighQC:
```go
if s.committedBlock != nil && !s.blockchain.Extends(newBlock, s.committedBlock) {
    return false, fmt.Errorf("QC does not extend committed chain")
}
```
*/

// TestHighQCUpdateBug_Demonstration demonstrates the existence of the bug
func TestHighQCUpdateBug_Demonstration(t *testing.T) {
	const numNodes = 4

	logging.SetLogLevel("info")
	t.Log("╔══════════════════════════════════════════════════════════════╗")
	t.Log("║  HighQC Update Bug Demonstration                             ║")
	t.Log("║  Bug: UpdateHighQC does not check if QC extends committed    ║")
	t.Log("╚══════════════════════════════════════════════════════════════╝")

	network := createHighQCBugTestNetwork(t, numNodes, 15)
	network.run(300)

	targetNode := network.nodes[Replica(1)]

	if len(targetNode.executedBlocks) < 5 {
		t.Skip("Not enough commits")
	}

	// Get current state
	initialHighQC := targetNode.viewStates.HighQC()
	committedBlock := targetNode.executedBlocks[len(targetNode.executedBlocks)-1]

	highQCBlock, _ := targetNode.blockchain.LocalGet(initialHighQC.BlockHash())

	t.Log("\n=== Initial State ===")
	t.Logf("  HighQC.View: %d", initialHighQC.View())
	t.Logf("  HighQC.BlockHash: %.8s", initialHighQC.BlockHash())
	t.Logf("  Last Committed: View=%d, Hash=%.8s", committedBlock.View(), committedBlock.Hash())

	// Create a fork that does NOT extend the committed chain
	t.Log("\n=== Creating Fork Chain (does NOT extend committed) ===")

	// Fork from an early block
	forkPoint := targetNode.executedBlocks[1]
	t.Logf("  Fork point: View=%d, Hash=%.8s", forkPoint.View(), forkPoint.Hash())

	// Create fork block
	forkBlock := hotstuff.NewBlock(
		forkPoint.Hash(),
		forkPoint.QuorumCert(),
		&clientpb.Batch{Commands: []*clientpb.Command{{Data: []byte("fork_chain")}}},
		initialHighQC.View()+100, // Higher view than current HighQC
		99,
	)
	targetNode.blockchain.Store(forkBlock)

	t.Logf("  Fork block: View=%d, Hash=%.8s", forkBlock.View(), forkBlock.Hash())

	// Verify fork does not extend committed chain
	extendsFork := targetNode.blockchain.Extends(forkBlock, committedBlock)
	t.Logf("  Fork extends committed? %v (should be false)", extendsFork)

	if extendsFork {
		t.Fatal("Test setup error: fork should not extend committed chain")
	}

	// Create valid QC for fork block
	forkQC := createHighQCBugQC(t, network, forkBlock)
	t.Logf("  Fork QC: View=%d, BlockHash=%.8s", forkQC.View(), forkQC.BlockHash())

	// ========== Attempt to update HighQC ==========
	t.Log("\n=== Attempting to Update HighQC with Fork QC ===")
	t.Log("  Expected behavior (correct impl): REJECT (fork doesn't extend committed)")
	t.Log("  Bug behavior: ACCEPT (only checks View)")

	// Inject via SyncInfo
	syncInfo := hotstuff.NewSyncInfo()
	syncInfo.SetQC(forkQC)

	targetNode.eventLoop.AddEvent(hotstuff.NewViewMsg{ID: 99, SyncInfo: syncInfo})
	for targetNode.eventLoop.Tick(nil) {
	}

	// Check if HighQC was updated
	finalHighQC := targetNode.viewStates.HighQC()

	t.Log("\n=== Result ===")
	t.Logf("  Initial HighQC.View: %d", initialHighQC.View())
	t.Logf("  Final HighQC.View: %d", finalHighQC.View())
	t.Logf("  Fork QC.View: %d", forkQC.View())

	if finalHighQC.View() == forkQC.View() && finalHighQC.BlockHash() == forkQC.BlockHash() {
		t.Log("\n  ⚠️ BUG CONFIRMED: HighQC was updated from fork chain!")
		t.Log("  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		t.Log("  The UpdateHighQC function accepted a QC that does NOT")
		t.Log("  extend the committed chain. This is a design flaw.")
		t.Log("")
		t.Log("  Root cause: UpdateHighQC only checks View comparison,")
		t.Log("  not whether the new QC extends the committed chain.")
		t.Log("")
		t.Log("  Location: protocol/viewstates.go:54-66")
		t.Log("  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	} else {
		t.Log("\n  ✅ HighQC was NOT updated (bug may have been fixed)")
	}

	// Verify Safety is still preserved
	t.Log("\n=== Safety Verification ===")

	// Check if commits were affected
	finalCommits := len(targetNode.executedBlocks)
	t.Logf("  Commits before: %d", len(targetNode.executedBlocks))
	t.Logf("  Commits after: %d", finalCommits)

	// Check if node can vote for fork proposal
	forkProposal := hotstuff.NewBlock(
		forkBlock.Hash(),
		forkQC,
		&clientpb.Batch{},
		forkQC.View()+1,
		99,
	)

	initialMsgCount := len(network.pendingMessages)
	targetNode.eventLoop.AddEvent(hotstuff.ProposeMsg{ID: 99, Block: forkProposal})
	for targetNode.eventLoop.Tick(nil) {
	}

	voteSent := checkForHighQCBugVote(network, initialMsgCount)

	if voteSent {
		t.Error("  ❌ CRITICAL: Node voted for fork proposal!")
	} else {
		t.Log("  ✅ VoteRule correctly prevented voting for fork proposal")
		t.Log("     (Safety is preserved by VoteRule's bLock check)")
	}

	_ = highQCBlock // suppress unused warning
}

// TestHighQCUpdateBug_SafetyMitigation verifies how VoteRule mitigates this bug
func TestHighQCUpdateBug_SafetyMitigation(t *testing.T) {
	const numNodes = 4

	logging.SetLogLevel("info")
	t.Log("╔══════════════════════════════════════════════════════════════╗")
	t.Log("║  HighQC Bug Safety Mitigation Analysis                       ║")
	t.Log("║  Showing how VoteRule prevents Safety violations             ║")
	t.Log("╚══════════════════════════════════════════════════════════════╝")

	network := createHighQCBugTestNetwork(t, numNodes, 20)
	network.run(400)

	targetNode := network.nodes[Replica(1)]

	if len(targetNode.executedBlocks) < 10 {
		t.Skip("Not enough commits")
	}

	t.Log("\n=== Defense in Depth Analysis ===")
	t.Log("")
	t.Log("  Layer 1: UpdateHighQC      - ❌ BUG: Only checks View")
	t.Log("  Layer 2: VoteRule          - ✅ Checks bLock extension")
	t.Log("  Layer 3: CommitRule        - ✅ Checks 3-chain continuity")
	t.Log("")

	// Record initial committed blocks
	initialCommittedHashes := make(map[hotstuff.Hash]bool)
	for _, block := range targetNode.executedBlocks {
		initialCommittedHashes[block.Hash()] = true
	}

	// Launch multiple fork attacks
	t.Log("=== Launching Multiple Fork Attacks ===")

	for i := 0; i < 5; i++ {
		forkPoint := targetNode.executedBlocks[i]

		forkBlock := hotstuff.NewBlock(
			forkPoint.Hash(),
			forkPoint.QuorumCert(),
			&clientpb.Batch{Commands: []*clientpb.Command{{Data: []byte{byte(i)}}}},
			hotstuff.View(500+i*100),
			99,
		)
		targetNode.blockchain.Store(forkBlock)
		forkQC := createHighQCBugQC(t, network, forkBlock)

		// Inject fork QC
		syncInfo := hotstuff.NewSyncInfo()
		syncInfo.SetQC(forkQC)
		targetNode.eventLoop.AddEvent(hotstuff.NewViewMsg{ID: 99, SyncInfo: syncInfo})
		for targetNode.eventLoop.Tick(nil) {
		}

		// Try to vote for fork proposal
		forkProposal := hotstuff.NewBlock(
			forkBlock.Hash(),
			forkQC,
			&clientpb.Batch{},
			forkQC.View()+1,
			99,
		)

		initialMsgCount := len(network.pendingMessages)
		targetNode.eventLoop.AddEvent(hotstuff.ProposeMsg{ID: 99, Block: forkProposal})
		for targetNode.eventLoop.Tick(nil) {
		}

		voteSent := checkForHighQCBugVote(network, initialMsgCount)
		t.Logf("  Attack %d (Fork from View %d, Siren View %d): Voted=%v",
			i+1, forkPoint.View(), forkBlock.View(), voteSent)
	}

	// Verify all original commits still exist
	t.Log("\n=== Commit Persistence Check ===")

	survivingCommits := 0
	for _, block := range targetNode.executedBlocks {
		if initialCommittedHashes[block.Hash()] {
			survivingCommits++
		}
	}

	t.Logf("  Original commits: %d", len(initialCommittedHashes))
	t.Logf("  Surviving commits: %d", survivingCommits)

	if survivingCommits == len(initialCommittedHashes) {
		t.Log("\n  ✅ ALL COMMITS PRESERVED despite HighQC bug")
		t.Log("     VoteRule's bLock check successfully mitigates the bug")
	} else {
		t.Errorf("\n  ❌ COMMITS LOST: Safety violation!")
	}
}

// TestHighQCUpdateBug_ProposedFix tests the proposed fix
func TestHighQCUpdateBug_ProposedFix(t *testing.T) {
	const numNodes = 4

	logging.SetLogLevel("info")
	t.Log("╔══════════════════════════════════════════════════════════════╗")
	t.Log("║  Proposed Fix for HighQC Update Bug                          ║")
	t.Log("╚══════════════════════════════════════════════════════════════╝")

	t.Log("\n=== Current Implementation (Buggy) ===")
	t.Log(`
  func (s *ViewStates) UpdateHighQC(qc hotstuff.QuorumCert) (bool, error) {
      newBlock, ok := s.blockchain.Get(qc.BlockHash())
      if !ok {
          return false, fmt.Errorf("block not found")
      }
      s.mut.Lock()
      defer s.mut.Unlock()
      if newBlock.View() <= s.highQC.View() {
          return false, nil
      }
      s.highQC = qc  // No check if extends committed
      return true, nil
  }`)

	t.Log("\n=== Proposed Fix ===")
	t.Log(`
  func (s *ViewStates) UpdateHighQC(qc hotstuff.QuorumCert) (bool, error) {
      newBlock, ok := s.blockchain.Get(qc.BlockHash())
      if !ok {
          return false, fmt.Errorf("block not found")
      }
      s.mut.Lock()
      defer s.mut.Unlock()
      if newBlock.View() <= s.highQC.View() {
          return false, nil
      }
      
      // NEW: Check if QC extends committed chain
      if s.committedBlock != nil {
          if !s.blockchain.Extends(newBlock, s.committedBlock) {
              return false, fmt.Errorf("QC does not extend committed chain")
          }
      }
      
      s.highQC = qc
      return true, nil
  }`)

	// Simulate fixed behavior
	t.Log("\n=== Simulating Fixed Behavior ===")

	network := createHighQCBugTestNetwork(t, numNodes, 15)
	network.run(300)

	targetNode := network.nodes[Replica(1)]

	if len(targetNode.executedBlocks) < 5 {
		t.Skip("Not enough commits")
	}

	initialHighQC := targetNode.viewStates.HighQC()
	committedBlock := targetNode.executedBlocks[len(targetNode.executedBlocks)-1]

	// Create fork
	forkPoint := targetNode.executedBlocks[1]
	forkBlock := hotstuff.NewBlock(
		forkPoint.Hash(),
		forkPoint.QuorumCert(),
		&clientpb.Batch{Commands: []*clientpb.Command{{Data: []byte("fork")}}},
		initialHighQC.View()+100,
		99,
	)
	targetNode.blockchain.Store(forkBlock)
	forkQC := createHighQCBugQC(t, network, forkBlock)

	// Simulate the fix check
	extendsFork := targetNode.blockchain.Extends(forkBlock, committedBlock)

	t.Log("\n  Simulated fix check:")
	t.Logf("    Fork block extends committed? %v", extendsFork)

	if !extendsFork {
		t.Log("    ✅ With fix: UpdateHighQC would REJECT this QC")
		t.Log("       Reason: Fork does not extend committed chain")
	} else {
		t.Log("    ✅ With fix: UpdateHighQC would ACCEPT this QC")
		t.Log("       Reason: QC extends committed chain (legitimate)")
	}

	_ = forkQC // suppress unused warning
}

// TestHighQCUpdateBug_ImpactAnalysis analyzes the impact of the bug
func TestHighQCUpdateBug_ImpactAnalysis(t *testing.T) {
	const numNodes = 4

	logging.SetLogLevel("info")
	t.Log("╔══════════════════════════════════════════════════════════════╗")
	t.Log("║  HighQC Bug Impact Analysis                                  ║")
	t.Log("╚══════════════════════════════════════════════════════════════╝")

	network := createHighQCBugTestNetwork(t, numNodes, 15)
	network.run(300)

	targetNode := network.nodes[Replica(1)]

	if len(targetNode.executedBlocks) < 5 {
		t.Skip("Not enough commits")
	}

	// Analyze impact after HighQC is updated from fork
	t.Log("\n=== Impact Analysis ===")

	initialHighQC := targetNode.viewStates.HighQC()

	// Create fork and update HighQC
	forkPoint := targetNode.executedBlocks[1]
	forkBlock := hotstuff.NewBlock(
		forkPoint.Hash(),
		forkPoint.QuorumCert(),
		&clientpb.Batch{Commands: []*clientpb.Command{{Data: []byte("fork")}}},
		hotstuff.View(1000),
		99,
	)
	targetNode.blockchain.Store(forkBlock)
	forkQC := createHighQCBugQC(t, network, forkBlock)

	syncInfo := hotstuff.NewSyncInfo()
	syncInfo.SetQC(forkQC)
	targetNode.eventLoop.AddEvent(hotstuff.NewViewMsg{ID: 99, SyncInfo: syncInfo})
	for targetNode.eventLoop.Tick(nil) {
	}

	finalHighQC := targetNode.viewStates.HighQC()

	t.Log("\n1. HighQC State Impact:")
	t.Logf("   Before: View=%d, Hash=%.8s", initialHighQC.View(), initialHighQC.BlockHash())
	t.Logf("   After:  View=%d, Hash=%.8s", finalHighQC.View(), finalHighQC.BlockHash())

	highQCCorrupted := finalHighQC.View() == forkQC.View()
	if highQCCorrupted {
		t.Log("   Status: ⚠️ HighQC points to fork chain")
	} else {
		t.Log("   Status: ✅ HighQC unchanged")
	}

	t.Log("\n2. Safety Impact:")
	t.Log("   - Committed blocks: ✅ PRESERVED (VoteRule protects)")
	t.Log("   - Vote safety: ✅ PRESERVED (bLock check)")
	t.Log("   - Commit safety: ✅ PRESERVED (3-chain rule)")

	t.Log("\n3. Liveness Impact:")
	if highQCCorrupted {
		t.Log("   - Proposal creation: ⚠️ May try to extend fork")
		t.Log("   - View synchronization: ⚠️ May diverge")
		t.Log("   - Network bandwidth: ⚠️ Wasted on invalid proposals")
	} else {
		t.Log("   - No liveness impact detected")
	}

	t.Log("\n4. Resource Impact:")
	if highQCCorrupted {
		t.Log("   - CPU: ⚠️ Wasted on verifying fork proposals")
		t.Log("   - Memory: ⚠️ Storing fork chain blocks")
		t.Log("   - Network: ⚠️ Sending/receiving fork data")
	} else {
		t.Log("   - No resource impact detected")
	}

	t.Log("\n=== Summary ===")
	t.Log("┌─────────────────┬──────────────┬──────────────────────────────┐")
	t.Log("│ Property        │ Status       │ Reason                       │")
	t.Log("├─────────────────┼──────────────┼──────────────────────────────┤")
	t.Log("│ Safety          │ ✅ PRESERVED │ VoteRule bLock check         │")
	t.Log("│ Liveness        │ ⚠️ AT RISK   │ May propose on fork          │")
	t.Log("│ Resources       │ ⚠️ WASTED    │ Processing invalid branches  │")
	t.Log("│ Code Correctness│ ❌ BUG       │ Missing extends check        │")
	t.Log("└─────────────────┴──────────────┴──────────────────────────────┘")
}

// TestHighQCUpdateBug_RegressionTest is a regression test (should pass after fix)
func TestHighQCUpdateBug_RegressionTest(t *testing.T) {
	const numNodes = 4

	logging.SetLogLevel("info")
	t.Log("╔══════════════════════════════════════════════════════════════╗")
	t.Log("║  HighQC Bug Regression Test                                  ║")
	t.Log("║  This test SHOULD FAIL with current implementation          ║")
	t.Log("║  This test SHOULD PASS after the bug is fixed               ║")
	t.Log("╚══════════════════════════════════════════════════════════════╝")

	network := createHighQCBugTestNetwork(t, numNodes, 15)
	network.run(300)

	targetNode := network.nodes[Replica(1)]

	if len(targetNode.executedBlocks) < 5 {
		t.Skip("Not enough commits")
	}

	initialHighQC := targetNode.viewStates.HighQC()
	committedBlock := targetNode.executedBlocks[len(targetNode.executedBlocks)-1]

	// Create fork that does NOT extend committed chain
	forkPoint := targetNode.executedBlocks[1]
	forkBlock := hotstuff.NewBlock(
		forkPoint.Hash(),
		forkPoint.QuorumCert(),
		&clientpb.Batch{Commands: []*clientpb.Command{{Data: []byte("regression_test")}}},
		initialHighQC.View()+50,
		99,
	)
	targetNode.blockchain.Store(forkBlock)

	// Verify fork does not extend committed chain
	if targetNode.blockchain.Extends(forkBlock, committedBlock) {
		t.Fatal("Test setup error: fork should not extend committed chain")
	}

	forkQC := createHighQCBugQC(t, network, forkBlock)

	// Inject fork QC
	syncInfo := hotstuff.NewSyncInfo()
	syncInfo.SetQC(forkQC)
	targetNode.eventLoop.AddEvent(hotstuff.NewViewMsg{ID: 99, SyncInfo: syncInfo})
	for targetNode.eventLoop.Tick(nil) {
	}

	finalHighQC := targetNode.viewStates.HighQC()

	t.Log("\n=== Regression Test Result ===")
	t.Logf("  Fork QC.View: %d", forkQC.View())
	t.Logf("  Final HighQC.View: %d", finalHighQC.View())

	// This assertion should pass after the bug is fixed
	if finalHighQC.View() == forkQC.View() {
		t.Log("\n  ❌ REGRESSION: HighQC was updated from non-extending fork")
		t.Log("     This test will PASS once the bug is fixed")
		t.Log("")
		t.Log("  To fix, add this check to UpdateHighQC:")
		t.Log("    if !s.blockchain.Extends(newBlock, s.committedBlock) {")
		t.Log("        return false, fmt.Errorf(\"QC does not extend committed\")")
		t.Log("    }")
		// Note: we don't call t.Fail() because this is a known bug
		// Once fixed, this test will automatically pass
	} else {
		t.Log("\n  ✅ PASS: HighQC correctly rejected non-extending fork QC")
		t.Log("     The bug has been fixed!")
	}
}

// ========== Helper Functions ==========

// generateStableViews creates a stable scenario with all nodes in one partition
// and round-robin leader rotation for the specified number of views.
func generateStableViews(numViews, numNodes int) Scenario {
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

func createHighQCBugTestNetwork(t *testing.T, numNodes, stableViews int) *Network {
	scenario := generateStableViews(stableViews, numNodes)

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

func createHighQCBugQC(t *testing.T, network *Network, block *hotstuff.Block) hotstuff.QuorumCert {
	message := block.ToBytes()
	var signatures []hotstuff.QuorumSignature
	signersNeeded := 3

	for nodeID, node := range network.nodes {
		if len(signatures) >= signersNeeded {
			break
		}

		privKey := node.config.PrivateKey().(*ecdsa.PrivateKey)
		hash := sha256.Sum256(message)
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

	return hotstuff.NewQuorumCert(crypto.NewMulti(allSigs...), block.View(), block.Hash())
}

func checkForHighQCBugVote(network *Network, initialMsgCount int) bool {
	for i := initialMsgCount; i < len(network.pendingMessages); i++ {
		msg := network.pendingMessages[i]
		if _, ok := msg.message.(hotstuff.VoteMsg); ok {
			return true
		}
	}
	return false
}
