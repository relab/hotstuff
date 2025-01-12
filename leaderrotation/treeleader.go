package leaderrotation

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/modules"
)

func init() {
	modules.RegisterModule("tree-leader", func() modules.LeaderRotation {
		return NewTreeLeader()
	})
}

type treeLeader struct {
	leader hotstuff.ID
	opts   *modules.Options
}

func (t *treeLeader) InitModule(mods *modules.Core) {
	mods.Get(&t.opts)
}
func NewTreeLeader() *treeLeader {
	return &treeLeader{leader: 1}
}

// GetLeader returns the id of the leader in the given view
func (t *treeLeader) GetLeader(_ hotstuff.View) hotstuff.ID {
	treeConfig := t.opts.TreeConfig()
	if treeConfig == nil {
		return 1
	}
	return treeConfig.TreePos()[0]
}
