package debug

import (
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/pipeline"
)

type CommitHaltEvent struct {
	OnPipe pipeline.Pipe
}

type CommandRejectedEvent struct {
	OnPipe pipeline.Pipe
	View   hotstuff.View
}