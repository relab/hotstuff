package debug

import (
	"github.com/relab/hotstuff"
)

type CommitHaltEvent struct {
	Instance hotstuff.Instance
}

type CommandRejectedEvent struct {
	Instance hotstuff.Instance
	View     hotstuff.View
}
