package twins

import (
	"testing"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/consensus/chainedhotstuff"
)

func TestBasicScenario(t *testing.T) {
	s := Scenario{
		Replicas: []hotstuff.ID{1, 2, 3, 4},
		Leaders:  []hotstuff.ID{1, 2, 3, 4},
		Nodes: []NodeID{
			{1, 1},
			{2, 2},
			{3, 3},
			{4, 4},
		},
		Rounds:    4,
		Byzantine: 0,
		ConsensusCtor: func() consensus.Consensus {
			return consensus.New(chainedhotstuff.New())
		},
		ViewTimeout: 100,
	}
	allNodesSet := make(NodeSet)
	for _, node := range s.Nodes {
		allNodesSet.Add(node)
	}
	s.Partitions = append(s.Partitions, []NodeSet{allNodesSet})
	s.Partitions = append(s.Partitions, []NodeSet{allNodesSet})
	s.Partitions = append(s.Partitions, []NodeSet{allNodesSet})
	s.Partitions = append(s.Partitions, []NodeSet{allNodesSet})

	safe, commits, err := ExecuteScenario(s)
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
