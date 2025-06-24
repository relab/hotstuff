package leaderrotation

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
)

const ModuleNameTree = "tree-leader"

type TreeBased struct {
	leader hotstuff.ID
	config *core.RuntimeConfig
}

func NewTreeBased(
	config *core.RuntimeConfig,
) *TreeBased {
	return &TreeBased{
		config: config,
		leader: 1,
	}
}

// GetLeader returns the id of the leader in the given view
func (t *TreeBased) GetLeader(_ hotstuff.View) hotstuff.ID {
	if !t.config.HasKauriTree() {
		return 1
	}
	return t.config.Tree().Root()
}

var _ LeaderRotation = (*TreeBased)(nil)
