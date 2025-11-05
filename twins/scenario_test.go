package twins

import (
	"testing"

	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/protocol/rules"
)

// TestPartitionedScenario checks if chained HotStuff will commit one block
// when all nodes are honest and the leader is in a separate partition.
func TestPartitionedScenario(t *testing.T) {
	s := Scenario{}
	allNodesSet := NewNodeSet(NodeID{1, 0}, NodeID{2, 0}, NodeID{3, 0}, NodeID{4, 0})
	partitionedSet := NewNodeSet(NodeID{1, 0}, NodeID{3, 0}, NodeID{4, 0})
	leaderSet := NewNodeSet(NodeID{2, 0})
	s = append(s, View{Leader: 1, Partitions: []NodeSet{allNodesSet}})
	s = append(s, View{Leader: 2, Partitions: []NodeSet{leaderSet, partitionedSet}})
	s = append(s, View{Leader: 3, Partitions: []NodeSet{allNodesSet}})
	s = append(s, View{Leader: 1, Partitions: []NodeSet{allNodesSet}})
	s = append(s, View{Leader: 1, Partitions: []NodeSet{allNodesSet}})
	logging.SetLogLevel("debug")
	result, err := ExecuteScenario(s, 4, 0, 100, rules.NameChainedHotStuff)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Safe {
		t.Errorf("Expected no safety violations")
	}
	if result.Commits != 1 {
		t.Error("Expected one commit")
	}
	t.Logf("Network log:\n%s", result.NetworkLog)
}

func TestPartitionedScenario2(t *testing.T) {
	s := Scenario{}
	allNodesSet := NewNodeSet(NodeID{1, 0}, NodeID{2, 0}, NodeID{3, 0}, NodeID{4, 0})
	partitionedSet := NewNodeSet(NodeID{1, 0}, NodeID{3, 0}, NodeID{4, 0})
	leaderSet := NewNodeSet(NodeID{2, 0})
	s = append(s, View{Leader: 1, Partitions: []NodeSet{allNodesSet}})
	s = append(s, View{Leader: 1, Partitions: []NodeSet{allNodesSet}})
	s = append(s, View{Leader: 2, Partitions: []NodeSet{leaderSet, partitionedSet}})
	s = append(s, View{Leader: 3, Partitions: []NodeSet{leaderSet, partitionedSet}})
	s = append(s, View{Leader: 3, Partitions: []NodeSet{leaderSet, partitionedSet}})
	s = append(s, View{Leader: 3, Partitions: []NodeSet{leaderSet, partitionedSet}})

	logging.SetLogLevel("debug")
	result, err := ExecuteScenario(s, 4, 0, 100, rules.NameChainedHotStuff)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Safe {
		t.Errorf("Expected no safety violations")
	}
	if result.Commits != 1 {
		t.Error("Expected one commit")
		for id, commits := range result.NodeCommits {
			t.Logf("Node %v commits:", id)
			for _, b := range commits {
				t.Logf("  %v", b)
			}
		}
	}

	t.Logf("Network log:\n%s", result.NetworkLog)
}

// TestBasicScenario checks if chained HotStuff will commit one block
// when all nodes are honest and the network is not partitioned.
func TestBasicScenario(t *testing.T) {
	s := Scenario{}
	allNodesSet := NewNodeSet(NodeID{1, 0}, NodeID{2, 0}, NodeID{3, 0}, NodeID{4, 0})
	s = append(s, View{Leader: 1, Partitions: []NodeSet{allNodesSet}})
	s = append(s, View{Leader: 1, Partitions: []NodeSet{allNodesSet}})
	s = append(s, View{Leader: 1, Partitions: []NodeSet{allNodesSet}})
	s = append(s, View{Leader: 1, Partitions: []NodeSet{allNodesSet}})
	logging.SetLogLevel("debug")
	result, err := ExecuteScenario(s, 4, 0, 100, rules.NameChainedHotStuff)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Safe {
		t.Errorf("Expected no safety violations")
	}
	if result.Commits != 1 {
		t.Error("Expected one commit")
	}
}

// TestBasicTwinsScenario checks if chained HotStuff will commit one block
// when one replica (not the leader) has a twin.
func TestBasicTwinsScenario(t *testing.T) {
	s := Scenario{}
	// With 1 twin: nodes with NetworkID 1 and 2 will be twins of replica 1.
	allNodesSet := NewNodeSet(NodeID{1, 1}, NodeID{1, 2}, NodeID{2, 0}, NodeID{3, 0}, NodeID{4, 0})
	s = append(s, View{Leader: 3, Partitions: []NodeSet{allNodesSet}})
	s = append(s, View{Leader: 3, Partitions: []NodeSet{allNodesSet}})
	s = append(s, View{Leader: 3, Partitions: []NodeSet{allNodesSet}})
	s = append(s, View{Leader: 3, Partitions: []NodeSet{allNodesSet}})
	logging.SetLogLevel("debug")
	result, err := ExecuteScenario(s, 4, 1, 100, rules.NameChainedHotStuff)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Safe {
		t.Errorf("Expected no safety violations")
	}
	if result.Commits != 1 {
		t.Error("Expected one commit")
		for id, commits := range result.NodeCommits {
			t.Logf("Node %v commits:", id)
			for _, b := range commits {
				t.Logf("  %v", b)
			}
		}
	}

	// t.Logf("Node logs:\n%s", result.NodeLogs[NodeID{1, 1}])
	t.Logf("Network log:\n%s", result.NetworkLog)
}

// TestTwinsScenarioNeeded checks if chained HotStuff will commit one block
// when one replica (not the leader) has a twin and the twins votes are needed
func TestTwinsScenarioNeeded(t *testing.T) {
	s := Scenario{}
	// With 1 twin: nodes with NetworkID 1 and 2 will be twins of replica 1.
	allNodesSet := NewNodeSet(NodeID{1, 1}, NodeID{1, 2}, NodeID{2, 0}, NodeID{3, 0}, NodeID{4, 0})
	BCD := NewNodeSet(NodeID{2, 0}, NodeID{1, 2}, NodeID{3, 0}) // node with NetworkID 2 is the twin of replica 1

	s = append(s, View{Leader: 3, Partitions: []NodeSet{allNodesSet}})
	s = append(s, View{Leader: 3, Partitions: []NodeSet{allNodesSet}})
	s = append(s, View{Leader: 3, Partitions: []NodeSet{BCD}})
	s = append(s, View{Leader: 3, Partitions: []NodeSet{BCD}})
	logging.SetLogLevel("info")
	result, err := ExecuteScenario(s, 4, 1, 100, rules.NameChainedHotStuff)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Safe {
		t.Errorf("Expected no safety violations")
	}
	if result.Commits != 1 {
		t.Error("Expected one commit")
		for id, commits := range result.NodeCommits {
			t.Logf("Node %v commits:", id)
			for _, b := range commits {
				t.Logf("  %v", b)
			}
		}
	}

	// t.Logf("Node logs:\n%s", result.NodeLogs[NodeID{1, 1}])
	t.Logf("Network log:\n%s", result.NetworkLog)
}

// TestTwinsScenarioRepNeeded checks if chained HotStuff will commit one block
// when one replica (not the leader) has a twin and the first twins votes are needed
func TestTwinsScenarioRepNeeded(t *testing.T) {
	s := Scenario{}
	// With 1 twin: nodes with NetworkID 1 and 2 will be twins of replica 1.
	allNodesSet := NewNodeSet(NodeID{1, 1}, NodeID{1, 2}, NodeID{2, 0}, NodeID{3, 0}, NodeID{4, 0})
	ACD := NewNodeSet(NodeID{2, 0}, NodeID{1, 1}, NodeID{3, 0}) // node with NetworkID 1 is the first twin of replica 1

	s = append(s, View{Leader: 3, Partitions: []NodeSet{allNodesSet}})
	s = append(s, View{Leader: 3, Partitions: []NodeSet{allNodesSet}})
	s = append(s, View{Leader: 3, Partitions: []NodeSet{ACD}})
	s = append(s, View{Leader: 3, Partitions: []NodeSet{ACD}})
	logging.SetLogLevel("debug")
	result, err := ExecuteScenario(s, 4, 1, 100, rules.NameChainedHotStuff)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Safe {
		t.Errorf("Expected no safety violations")
	}
	if result.Commits != 1 {
		t.Error("Expected one commit")
		for id, commits := range result.NodeCommits {
			t.Logf("Node %v commits:", id)
			for _, b := range commits {
				t.Logf("  %v", b)
			}
		}
	}

	if false { //set to true to print the log
		t.Fail()
		t.Logf("Network log:\n%s", result.NetworkLog)
	}

}

func TestSafetyWithTwins(t *testing.T) {
	s := Scenario{}
	// With 1 twin: nodes with NetworkID 1 and 2 will be twins of replica 1.
	// twinA := NewNodeSet(NodeID{1, 1})
	// twinB := NewNodeSet(NodeID{1, 2})
	allNodesSet := NewNodeSet(NodeID{1, 1}, NodeID{1, 2}, NodeID{2, 0}, NodeID{3, 0}, NodeID{4, 0})
	noA := NewNodeSet(NodeID{1, 2}, NodeID{2, 0}, NodeID{3, 0})
	noB := NewNodeSet(NodeID{1, 1}, NodeID{2, 0}, NodeID{3, 0}, NodeID{4, 0})

	s = append(s, View{Leader: 2, Partitions: []NodeSet{allNodesSet}})
	s = append(s, View{Leader: 2, Partitions: []NodeSet{noA}})
	s = append(s, View{Leader: 1, Partitions: []NodeSet{noB}})
	s = append(s, View{Leader: 1, Partitions: []NodeSet{noA}})
	s = append(s, View{Leader: 1, Partitions: []NodeSet{noB}})
	s = append(s, View{Leader: 1, Partitions: []NodeSet{noA}})
	s = append(s, View{Leader: 1, Partitions: []NodeSet{noB}})
	s = append(s, View{Leader: 1, Partitions: []NodeSet{noA}})
	s = append(s, View{Leader: 1, Partitions: []NodeSet{noB}})
	s = append(s, View{Leader: 1, Partitions: []NodeSet{noB}})
	logging.SetLogLevel("debug")
	result, err := ExecuteScenario(s, 4, 1, 100, rules.NameChainedHotStuff)
	if err != nil {
		t.Fatal(err)
	}

	if !result.Safe {
		t.Errorf("Expected no safety violations")
	}
	if result.Commits != 0 {
		t.Error("Expected no commit")
		for id, commits := range result.NodeCommits {
			t.Logf("Node %v commits:", id)
			for _, b := range commits {
				t.Logf("  %v", b)
			}
		}
	}


}
