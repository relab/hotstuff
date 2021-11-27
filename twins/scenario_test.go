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
	s = append(s, View{Leader: 1, Partitions: []NodeSet{allNodesSet}})
	s = append(s, View{Leader: 1, Partitions: []NodeSet{allNodesSet}})
	s = append(s, View{Leader: 1, Partitions: []NodeSet{allNodesSet}})
	s = append(s, View{Leader: 1, Partitions: []NodeSet{allNodesSet}})

	safe, commits, err := ExecuteScenario(s, 4, 0, "chainedhotstuff")
	if err != nil {
		t.Fatal(err)
	}

	if !safe {
		t.Errorf("Expected no safety violations")
	}

	if commits != 1 {
		t.Error("Expected one commit")
	}
}
