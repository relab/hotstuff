package treeleader

import (
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol/synchronizer/viewduration"
)

const ModuleName = "tree-leader"

type treeLeader struct {
	leader       hotstuff.ID
	config       *core.RuntimeConfig
	viewDuration modules.ViewDuration
}

func New(
	viewDuration time.Duration,
	config *core.RuntimeConfig,
) modules.LeaderRotation {
	return &treeLeader{
		config:       config,
		leader:       1,
		viewDuration: viewduration.NewFixed(viewDuration),
	}
}

func (t *treeLeader) ViewDuration() modules.ViewDuration {
	return t.viewDuration
}

// GetLeader returns the id of the leader in the given view
func (t *treeLeader) GetLeader(_ hotstuff.View) hotstuff.ID {
	if !t.config.HasKauriTree() {
		return 1
	}
	return t.config.Tree().Root()
}
