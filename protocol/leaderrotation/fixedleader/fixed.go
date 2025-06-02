package fixedleader

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/modules"
	"github.com/relab/hotstuff/protocol/synchronizer/viewduration"
)

const ModuleName = "fixed"

type fixed struct {
	leader       hotstuff.ID
	viewDuration modules.ViewDuration
}

func (f fixed) ViewDuration() modules.ViewDuration {
	return f.viewDuration
}

// GetLeader returns the id of the leader in the given view
func (f fixed) GetLeader(_ hotstuff.View) hotstuff.ID {
	return f.leader
}

// NewFixed returns a new fixed-leader leader rotation implementation.
func New(
	leader hotstuff.ID,
	opt viewduration.Params,
) modules.LeaderRotation {
	return fixed{
		leader:       leader,
		viewDuration: viewduration.NewDynamic(opt),
	}
}
