package clientpb

// MessageID is a unique identifier for a command.
type MessageID struct {
	ClientID       uint32
	SequenceNumber uint64
}

// ID returns the unique identifier for the command.
func (x *Command) ID() MessageID {
	return MessageID{
		ClientID:       x.GetClientID(),
		SequenceNumber: x.GetSequenceNumber(),
	}
}
