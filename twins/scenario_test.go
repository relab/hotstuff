package twins

import (
	"testing"

	"github.com/relab/hotstuff/core/logging"
	_ "github.com/relab/hotstuff/protocol/rules/chainedhotstuff"
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
	logging.SetLogLevel("debug")
	result, err := ExecuteScenario(s, 4, 0, 100, "chainedhotstuff")
	if err != nil {
		t.Log(err)
		t.Log("Logging debug...")
		t.Log(result.NetworkLog)
		t.Fail()
		return
	}

	if !result.Safe {
		t.Errorf("Expected no safety violations")
	}

	if result.Commits != 1 {
		t.Error("Expected one commit")
	}
	t.Log(result.NetworkLog)
}
