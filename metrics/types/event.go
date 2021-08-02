package types

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

func newEvent(client bool, id uint32, timestamp time.Time) *Event {
	return &Event{
		ID:        id,
		Client:    client,
		Timestamp: timestamppb.New(timestamp),
	}
}

// NewReplicaEvent creates a new replica event.
func NewReplicaEvent(id uint32, timestamp time.Time) *Event {
	return newEvent(false, id, timestamp)
}

// NewClientEvent creates a new client event.
func NewClientEvent(id uint32, timestamp time.Time) *Event {
	return newEvent(true, id, timestamp)
}

// TickEvent is sent when new measurements should be recorded.
type TickEvent struct {
	// The time when the previous tick happened.
	LastTick time.Time
}
