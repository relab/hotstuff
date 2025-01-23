package leaderrotation

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core"
	"github.com/relab/hotstuff/modules"
)

func init() {
	modules.RegisterModule("tree-leader", func() modules.LeaderRotation {
		return NewTreeLeader()
	})
}

type treeLeader struct {
	leader hotstuff.ID
	opts   *core.Options
}

func (t *treeLeader) InitModule(mods *core.Core) {
	mods.Get(&t.opts)
}

func NewTreeLeader() *treeLeader {
	return &treeLeader{leader: 1}
}

// GetLeader returns the id of the leader in the given view
func (t *treeLeader) GetLeader(_ hotstuff.View) hotstuff.ID {
	if t.opts == nil {
		panic("oops")
	}

	treeConfig := t.opts.TreeConfig()
	if treeConfig == nil {
		return 1
	}
	return treeConfig.TreePos()[0]
}
