package twins

import (
	"testing"
	"time"

	_ "github.com/relab/hotstuff/consensus/chainedhotstuff"
)

func TestBasicScenario(t *testing.T) {
	s := Scenario{
		Nodes: []NodeID{
			{1, 1},
			{2, 2},
			{3, 3},
			{4, 4},
		},
	}
	allNodesSet := make(NodeSet)
	for _, node := range s.Nodes {
		allNodesSet.Add(node)
	}
	s.Views = append(s.Views, View{Leader: 1, PartitionScenario: []NodeSet{allNodesSet}})
	s.Views = append(s.Views, View{Leader: 1, PartitionScenario: []NodeSet{allNodesSet}})
	s.Views = append(s.Views, View{Leader: 1, PartitionScenario: []NodeSet{allNodesSet}})
	s.Views = append(s.Views, View{Leader: 1, PartitionScenario: []NodeSet{allNodesSet}})

	safe, commits, err := ExecuteScenario(s, "chainedhotstuff", 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}

	if !safe {
		t.Errorf("Expected scenario no safety violations")
	}

	if commits != 1 {
		t.Error("Expected one commit")
	}
}
