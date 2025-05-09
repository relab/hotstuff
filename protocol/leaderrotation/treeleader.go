package leaderrotation

import (
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol/synchronizer/viewduration"
)

const TreeLeaderModuleName = "tree-leader"

type treeLeader struct {
	leader       hotstuff.ID
	globals      *core.Globals
	viewDuration modules.ViewDuration
}

func NewTreeLeader(
	viewDuration time.Duration,
	globals *core.Globals,
) modules.LeaderRotation {
	return &treeLeader{
		globals:      globals,
		leader:       1,
		viewDuration: viewduration.NewFixed(viewDuration),
	}
}

func (t *treeLeader) ViewDuration() modules.ViewDuration {
	return t.viewDuration
}

// GetLeader returns the id of the leader in the given view
func (t *treeLeader) GetLeader(_ hotstuff.View) hotstuff.ID {
	if t.globals == nil {
		panic("oops")
	}

	if !t.globals.ShouldUseTree() {
		return 1
	}
	return t.globals.Tree().Root()
}
