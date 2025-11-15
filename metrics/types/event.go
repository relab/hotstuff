// Package types defines various types for metrics collection.
package types

import (
	"time"

	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/client"
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
func NewReplicaEvent(id hotstuff.ID, timestamp time.Time) *Event {
	return newEvent(false, uint32(id), timestamp)
}

// NewClientEvent creates a new client event.
func NewClientEvent(id client.ID, timestamp time.Time) *Event {
	return newEvent(true, uint32(id), timestamp)
}

// TickEvent is sent when new measurements should be recorded.
type TickEvent struct {
	// The time when the previous tick happened.
	LastTick time.Time
}
