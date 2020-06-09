package pacemaker

import (
	"testing"

	"github.com/relab/hotstuff/config"
)

func TestRRGetLeader(t *testing.T) {
	pm := NewRoundRobin(1, []config.ReplicaID{1, 2, 3, 4}, 0)
	testCases := []struct {
		height int
		leader config.ReplicaID
	}{
		{0, 1}, {1, 2}, {2, 3}, {3, 4}, {4, 1},
	}
	for _, testCase := range testCases {
		if leader := pm.GetLeader(testCase.height); leader != testCase.leader {
			t.Errorf("Incorrect leader for view %d: got: %d, want: %d", testCase.height, leader, testCase.leader)
		}
	}
}
