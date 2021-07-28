package types

import (
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func newEvent(client bool, id uint32, timestamp time.Time, data proto.Message) (*Event, error) {
	any, err := anypb.New(data)
	if err != nil {
		return nil, err
	}
	return &Event{
		ID:        id,
		Client:    client,
		Timestamp: timestamppb.New(timestamp),
		Data:      any,
	}, nil
}

// NewReplicaEvent creates a new replica event.
func NewReplicaEvent(id uint32, timestamp time.Time, data proto.Message) (*Event, error) {
	return newEvent(false, id, timestamp, data)
}

// NewClientEvent creates a new client event.
func NewClientEvent(id uint32, timestamp time.Time, data proto.Message) (*Event, error) {
	return newEvent(true, id, timestamp, data)
}

// TickEvent is sent when new measurements should be recorded.
type TickEvent struct {
	// the time when the tick happened
	Timestamp time.Time
	// the time elapsed since last tick
	Elapsed time.Duration
}
