package twins

import (
	"testing"

	_ "github.com/relab/hotstuff/protocol/rules/chainedhotstuff"
)

func TestPartionedScenario(t *testing.T) {
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
	result, err := ExecuteScenario(s, 4, 0, 100, "chainedhotstuff")
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
	result, err := ExecuteScenario(s, 4, 0, 100, "chainedhotstuff")
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
