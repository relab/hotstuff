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
	allNodesSet := make(NodeSet)
	for i := 1; i <= 4; i++ {
		allNodesSet.Add(uint32(i))
	}
	partitionedSet := make(NodeSet)
	partitionedSet.Add(1)
	partitionedSet.Add(3)
	partitionedSet.Add(4)
	leaderSet := make(NodeSet)
	leaderSet.Add(2)
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

// TestBasicScenario checks if chained HotStuff will commit one block
// when all nodes are honest and the network is not partitioned.
func TestBasicScenario(t *testing.T) {
	s := Scenario{}
	allNodesSet := make(NodeSet)
	for i := 1; i <= 4; i++ {
		allNodesSet.Add(uint32(i))
	}
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
