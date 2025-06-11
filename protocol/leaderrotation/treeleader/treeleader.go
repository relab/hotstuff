package treeleader

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/modules"
)

const ModuleName = "tree-leader"

type treeLeader struct {
	leader hotstuff.ID
	config *core.RuntimeConfig
}

func New(
	config *core.RuntimeConfig,
) modules.LeaderRotation {
	return &treeLeader{
		config: config,
		leader: 1,
	}
}

// GetLeader returns the id of the leader in the given view
func (t *treeLeader) GetLeader(_ hotstuff.View) hotstuff.ID {
	if !t.config.HasKauriTree() {
		return 1
	}
	return t.config.Tree().Root()
}
