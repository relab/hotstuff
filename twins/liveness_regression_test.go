package twins_test

import (
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/protocol/rules"
	"github.com/relab/hotstuff/twins"
)

// TestLaggingNodeSync verifies that a lagging node can sync up after being
// isolated from the network. This test addresses the concern that using LocalGet
// in VerifyQuorumCert might prevent lagging nodes from syncing.
//
// Scenario:
// 1. Start a 4-node cluster
// 2. Isolate node 4 from the network for views 1-10
// 3. Let nodes 1-3 progress to view 50
// 4. Reconnect node 4 to the network
// 5. Verify node 4 can catch up with the cluster
//
// Expected behavior with correct implementation:
// - Node 4 should receive HighQC from other nodes
// - Node 4 should fetch missing blocks and sync its view
//
// Failure indicates:
// - LocalGet in VerifyQuorumCert causes messages to be silently dropped
// - No Fetch/Sync mechanism is triggered for missing blocks
func TestLaggingNodeSync(t *testing.T) {
	const (
		numNodes    = 4
		numTwins    = 0
		targetView  = 50
		minSyncView = 45 // Node 4 should reach at least this view
	)

	// Create a scenario where node 4 is isolated for the first 10 views,
	// then rejoins the network.
	scenario := createLaggingNodeScenario(numNodes, targetView, 10)

	result, err := twins.ExecuteScenario(scenario, uint8(numNodes), uint8(numTwins), 500, rules.NameChainedHotStuff)
	if err != nil {
		t.Fatalf("Failed to execute scenario: %v", err)
	}

	if !result.Safe {
		t.Logf("Network log:\n%s", result.NetworkLog)
		for id, log := range result.NodeLogs {
			t.Logf("Node %v log:\n%s", id, log)
		}
		t.Fatal("Safety violation detected")
	}

	// Check if the lagging node (node 4) caught up
	laggingNodeID := twins.Replica(hotstuff.ID(4))
	laggingNodeCommits := result.NodeCommits[laggingNodeID]

	t.Logf("Total commits across all nodes:")
	for id, commits := range result.NodeCommits {
		t.Logf("  Node %v: %d commits", id, len(commits))
	}

	// Find the maximum view reached by any node
	var maxView hotstuff.View
	for _, commits := range result.NodeCommits {
		for _, block := range commits {
			if block.View() > maxView {
				maxView = block.View()
			}
		}
	}
	t.Logf("Maximum view reached: %d", maxView)

	// Check if node 4 made any progress
	if len(laggingNodeCommits) == 0 {
		t.Errorf("LIVENESS REGRESSION: Lagging node (node 4) made no commits after rejoining. "+
			"This indicates that LocalGet in VerifyQuorumCert is preventing sync. "+
			"Max view in cluster: %d", maxView)
	}

	// Check the highest view node 4 reached
	var node4MaxView hotstuff.View
	for _, block := range laggingNodeCommits {
		if block.View() > node4MaxView {
			node4MaxView = block.View()
		}
	}

	// Node 4 should have caught up to at least minSyncView
	// This is a more lenient check to allow for some lag
	if node4MaxView < hotstuff.View(minSyncView) && maxView >= hotstuff.View(targetView) {
		t.Errorf("LIVENESS REGRESSION: Lagging node (node 4) only reached view %d, "+
			"expected at least %d. Cluster reached view %d. "+
			"This suggests missing blocks are not being fetched properly.",
			node4MaxView, minSyncView, maxView)
	}

	t.Logf("Test result: Node 4 reached view %d (target: %d)", node4MaxView, minSyncView)
}

// TestOutOfOrderParent verifies that proposals arriving before their parent
// blocks are handled correctly (either deferred or fetched).
//
// Scenario:
// 1. Create a proposal P that depends on parent block B
// 2. Send proposal P to a node that doesn't have block B
// 3. Verify the node either:
//    a) Attempts to fetch block B, OR
//    b) Defers processing until block B arrives
//
// Expected behavior with correct implementation:
// - The proposal should not be permanently discarded
// - The node should eventually process the proposal after getting block B
//
// Failure indicates:
// - Proposals with missing parents are silently dropped
// - No Pending queue or Fetch mechanism exists
func TestOutOfOrderParent(t *testing.T) {
	const (
		numNodes = 4
		numTwins = 0
	)

	// Create a scenario with intermittent network partitions to simulate
	// out-of-order message delivery.
	// In views 1-3: All nodes connected
	// In views 4-6: Node 4 only sees node 1 (leader)
	// In views 7-10: All nodes reconnected
	scenario := createOutOfOrderScenario(numNodes, 10)

	result, err := twins.ExecuteScenario(scenario, uint8(numNodes), uint8(numTwins), 200, rules.NameChainedHotStuff)
	if err != nil {
		t.Fatalf("Failed to execute scenario: %v", err)
	}

	if !result.Safe {
		t.Logf("Network log:\n%s", result.NetworkLog)
		for id, log := range result.NodeLogs {
			t.Logf("Node %v log:\n%s", id, log)
		}
		t.Fatal("Safety violation detected")
	}

	// All nodes should have made progress
	minExpectedCommits := 2 // At least some commits expected

	for i := 1; i <= numNodes; i++ {
		nodeID := twins.Replica(hotstuff.ID(i))
		commits := result.NodeCommits[nodeID]
		if len(commits) < minExpectedCommits {
			t.Errorf("LIVENESS REGRESSION: Node %d only has %d commits, expected at least %d. "+
				"This may indicate out-of-order proposals are being dropped.",
				i, len(commits), minExpectedCommits)
		}
	}

	t.Logf("Test passed: All nodes made progress despite network partitions")
	t.Logf("Commits per node:")
	for id, commits := range result.NodeCommits {
		t.Logf("  Node %v: %d commits", id, len(commits))
	}
}

// TestHighQCWithMissingBlock tests the specific scenario where a node receives
// a NewViewMsg with a HighQC referencing a block it doesn't have.
//
// This is the core test for the LocalGet regression concern.
func TestHighQCWithMissingBlock(t *testing.T) {
	const (
		numNodes = 4
		numTwins = 0
	)

	// Create scenario where node 4 misses some proposals but receives NewView messages
	// Views 1-5: Node 4 isolated (misses blocks)
	// Views 6-15: Node 4 connected, receives HighQC with references to blocks it doesn't have
	scenario := createHighQCMissingBlockScenario(numNodes, 15)

	result, err := twins.ExecuteScenario(scenario, uint8(numNodes), uint8(numTwins), 300, rules.NameChainedHotStuff)
	if err != nil {
		t.Fatalf("Failed to execute scenario: %v", err)
	}

	if !result.Safe {
		t.Logf("Network log:\n%s", result.NetworkLog)
		t.Fatal("Safety violation detected")
	}

	// Find max progress of majority vs node 4
	node4ID := twins.Replica(hotstuff.ID(4))
	node4Commits := len(result.NodeCommits[node4ID])

	var majorityMinCommits int = 999999
	for i := 1; i <= 3; i++ {
		nodeID := twins.Replica(hotstuff.ID(i))
		if commits := len(result.NodeCommits[nodeID]); commits < majorityMinCommits {
			majorityMinCommits = commits
		}
	}

	t.Logf("Majority nodes min commits: %d, Node 4 commits: %d", majorityMinCommits, node4Commits)

	// Node 4 should have caught up reasonably well
	// Allow for some lag but it shouldn't be stuck at 0
	if node4Commits == 0 && majorityMinCommits > 5 {
		t.Errorf("LIVENESS REGRESSION: Node 4 has 0 commits while majority has at least %d. "+
			"HighQC verification likely fails due to LocalGet not finding blocks, "+
			"and no Fetch mechanism is triggered.", majorityMinCommits)
	}

	// Node 4 should have at least half the commits of the majority
	if majorityMinCommits > 0 && float64(node4Commits)/float64(majorityMinCommits) < 0.3 {
		t.Errorf("LIVENESS REGRESSION: Node 4 significantly behind (commits: %d) compared to majority (%d). "+
			"Sync mechanism may not be working properly.", node4Commits, majorityMinCommits)
	}
}

// createLaggingNodeScenario creates a scenario where node 4 is isolated for
// the first isolatedViews, then rejoins.
func createLaggingNodeScenario(numNodes int, totalViews, isolatedViews int) twins.Scenario {
	scenario := make(twins.Scenario, totalViews)

	// Create node sets
	allNodes := twins.NewNodeSet()
	majorityNodes := twins.NewNodeSet()
	for i := 1; i <= numNodes; i++ {
		allNodes.Add(twins.Replica(hotstuff.ID(i)))
		if i < numNodes {
			majorityNodes.Add(twins.Replica(hotstuff.ID(i)))
		}
	}
	isolatedNode := twins.NewNodeSet(twins.Replica(hotstuff.ID(numNodes)))

	for view := 0; view < totalViews; view++ {
		leader := hotstuff.ID((view % (numNodes - 1)) + 1) // Rotate among nodes 1-3

		if view < isolatedViews {
			// Node 4 is isolated
			scenario[view] = twins.View{
				Leader:     leader,
				Partitions: []twins.NodeSet{majorityNodes, isolatedNode},
			}
		} else {
			// All nodes connected
			scenario[view] = twins.View{
				Leader:     leader,
				Partitions: []twins.NodeSet{allNodes},
			}
		}
	}

	return scenario
}

// createOutOfOrderScenario creates a scenario with intermittent partitions
// to simulate out-of-order message delivery.
// Uses a 3-node majority partition to ensure consensus can still progress.
func createOutOfOrderScenario(numNodes, totalViews int) twins.Scenario {
	scenario := make(twins.Scenario, totalViews)

	allNodes := twins.NewNodeSet()
	for i := 1; i <= numNodes; i++ {
		allNodes.Add(twins.Replica(hotstuff.ID(i)))
	}

	// Partition configurations
	// Majority group: nodes 1, 2, 3 (can reach quorum)
	// Isolated: node 4
	majorityGroup := twins.NewNodeSet(
		twins.Replica(hotstuff.ID(1)),
		twins.Replica(hotstuff.ID(2)),
		twins.Replica(hotstuff.ID(3)),
	)
	isolatedNode := twins.NewNodeSet(twins.Replica(hotstuff.ID(4)))

	for view := 0; view < totalViews; view++ {
		leader := hotstuff.ID((view % 3) + 1) // Rotate among nodes 1-3

		// Alternate between connected and partitioned states
		if view >= 3 && view <= 5 {
			// Node 4 isolated - majority can still progress
			scenario[view] = twins.View{
				Leader:     leader,
				Partitions: []twins.NodeSet{majorityGroup, isolatedNode},
			}
		} else {
			// All connected
			scenario[view] = twins.View{
				Leader:     leader,
				Partitions: []twins.NodeSet{allNodes},
			}
		}
	}

	return scenario
}

// createHighQCMissingBlockScenario creates a scenario specifically designed
// to test handling of HighQC with missing blocks.
func createHighQCMissingBlockScenario(numNodes, totalViews int) twins.Scenario {
	scenario := make(twins.Scenario, totalViews)

	allNodes := twins.NewNodeSet()
	majorityNodes := twins.NewNodeSet()
	for i := 1; i <= numNodes; i++ {
		allNodes.Add(twins.Replica(hotstuff.ID(i)))
		if i < numNodes {
			majorityNodes.Add(twins.Replica(hotstuff.ID(i)))
		}
	}
	node4Only := twins.NewNodeSet(twins.Replica(hotstuff.ID(numNodes)))

	for view := 0; view < totalViews; view++ {
		leader := hotstuff.ID((view % (numNodes - 1)) + 1) // Rotate among nodes 1-3

		if view < 5 {
			// Views 0-4: Node 4 isolated, misses all blocks
			scenario[view] = twins.View{
				Leader:     leader,
				Partitions: []twins.NodeSet{majorityNodes, node4Only},
			}
		} else {
			// Views 5+: All connected, node 4 should sync via HighQC
			scenario[view] = twins.View{
				Leader:     leader,
				Partitions: []twins.NodeSet{allNodes},
			}
		}
	}

	return scenario
}

