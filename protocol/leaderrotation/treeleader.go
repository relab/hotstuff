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
	opts         *core.Options
	viewDuration modules.ViewDuration
}

func NewTreeLeader(
	viewDuration time.Duration,
	opts *core.Options,
) modules.LeaderRotation {
	return &treeLeader{
		opts:         opts,
		leader:       1,
		viewDuration: viewduration.NewFixed(viewDuration),
	}
}

func (t *treeLeader) ViewDuration() modules.ViewDuration {
	return t.viewDuration
}

// GetLeader returns the id of the leader in the given view
func (t *treeLeader) GetLeader(_ hotstuff.View) hotstuff.ID {
	if t.opts == nil {
		panic("oops")
	}

	if !t.opts.ShouldUseTree() {
		return 1
	}
	return t.opts.Tree().Root()
}
