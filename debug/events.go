package debug

import (
	"github.com/relab/hotstuff"
)

type CommitHaltEvent struct {
	OnPipe hotstuff.Instance
}

type CommandRejectedEvent struct {
	OnPipe hotstuff.Instance
	View   hotstuff.View
}
