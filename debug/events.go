package debug

import (
	"github.com/relab/hotstuff"
)

type CommitHaltEvent struct {
	Pipe hotstuff.Pipe
}

type CommandRejectedEvent struct {
	Pipe hotstuff.Pipe
	View hotstuff.View
}
