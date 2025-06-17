package treeleader

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/modules"
)

const ModuleName = "tree-leader"

type TreeLeader struct {
	leader hotstuff.ID
	config *core.RuntimeConfig
}

func New(
	config *core.RuntimeConfig,
) *TreeLeader {
	return &TreeLeader{
		config: config,
		leader: 1,
	}
}

// GetLeader returns the id of the leader in the given view
func (t *TreeLeader) GetLeader(_ hotstuff.View) hotstuff.ID {
	if !t.config.HasKauriTree() {
		return 1
	}
	return t.config.Tree().Root()
}

var _ modules.LeaderRotation = (*TreeLeader)(nil)
