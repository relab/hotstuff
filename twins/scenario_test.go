package twins

import (
	"testing"

	_ "github.com/relab/hotstuff/consensus/chainedhotstuff"
)

func TestBasicScenario(t *testing.T) {
	s := Scenario{}
	allNodesSet := make(NodeSet)
	for i := 1; i <= 4; i++ {
		allNodesSet.Add(uint32(i))
	}
	lonely := make(NodeSet)
	lonely.Add(3)

	others := make(NodeSet)
	for i := 1; i <= 4; i++ {
		if !lonely.Contains(uint32(i)) {
			others.Add(uint32(i))
		}
	}

	s = append(s, View{Leader: 1, Partitions: []NodeSet{allNodesSet}})
	s = append(s, View{Leader: 2, Partitions: []NodeSet{allNodesSet}})
	s = append(s, View{Leader: 3, Partitions: []NodeSet{lonely, others}})
	s = append(s, View{Leader: 4, Partitions: []NodeSet{allNodesSet}})
	// s = append(s, View{Leader: 1, Partitions: []NodeSet{allNodesSet}})
	// s = append(s, View{Leader: 2, Partitions: []NodeSet{allNodesSet}})
	// s = append(s, View{Leader: 3, Partitions: []NodeSet{allNodesSet}})

	result, err := ExecuteScenario(s, 4, 0, 100, "chainedhotstuff")
	if err != nil {
		t.Fatal(err)
	}

	if !result.Safe {
		t.Errorf("Expected no safety violations")
	}

	if result.Commits < 1 {
		t.Error("Expected at least one commit")
	}

	if result.Commits > 1 {
		t.Error("Expected only one commit")
	}
}
