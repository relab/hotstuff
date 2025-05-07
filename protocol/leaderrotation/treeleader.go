package leaderrotation

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
)

const TreeLeaderModuleName = "tree-leader"

type treeLeader struct {
	leader hotstuff.ID
	opts   *core.Options
}

func NewTreeLeader(opts *core.Options) *treeLeader {
	t := &treeLeader{opts: opts, leader: 1}
	return t
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
