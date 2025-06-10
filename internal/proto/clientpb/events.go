package clientpb

// ExecuteEvent is raised when executing a batch of commands.
type ExecuteEvent struct {
	Batch *Batch
}

// AbortEvent is raised when a batch of commands is invalid and is canceled.
type AbortEvent struct {
	Batch *Batch
}
