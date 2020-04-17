package pacemaker

import (
	"testing"

	"github.com/relab/hotstuff"
)

func TestRRGetLeader(t *testing.T) {
	pm := NewRoundRobin(nil, 1, []hotstuff.ReplicaID{1, 2, 3, 4}, 0)
	testCases := []struct {
		height int
		leader hotstuff.ReplicaID
	}{
		{1, 1}, {2, 2}, {3, 3}, {4, 4}, {5, 1},
	}
	for _, testCase := range testCases {
		if leader := pm.getLeader(testCase.height); leader != testCase.leader {
			t.Errorf("Incorrect leader for view %d: got: %d, want: %d", testCase.height, leader, testCase.leader)
		}
	}
}
