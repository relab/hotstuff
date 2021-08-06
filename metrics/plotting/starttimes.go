package plotting

import (
	"time"

	"github.com/relab/hotstuff/metrics/types"
)

// StartTimes collects the start times for each client or replica.
type StartTimes struct {
	clients  map[uint32]time.Time
	replicas map[uint32]time.Time
}

// NewStartTimes returns a new StartTimes instance.
func NewStartTimes() StartTimes {
	return StartTimes{
		clients:  make(map[uint32]time.Time),
		replicas: make(map[uint32]time.Time),
	}
}

// Add adds an event.
func (s *StartTimes) Add(msg interface{}) {
	startTime, ok := msg.(*types.StartEvent)
	if !ok {
		return
	}

	if startTime.GetEvent().GetClient() {
		s.clients[startTime.GetEvent().GetID()] = startTime.GetEvent().GetTimestamp().AsTime()
	} else {
		s.replicas[startTime.GetEvent().GetID()] = startTime.GetEvent().GetTimestamp().AsTime()
	}
}

// Client returns the start time of the client with the specified id.
func (s *StartTimes) Client(id uint32) (t time.Time, ok bool) {
	t, ok = s.clients[id]
	return
}

// ClientOffset returns the time offset from the client's start time.
func (s *StartTimes) ClientOffset(id uint32, t time.Time) (offset time.Duration, ok bool) {
	startTime, ok := s.clients[id]
	if !ok {
		return 0, false
	}
	return t.Sub(startTime), true
}

// Replica returns the start time of the replica with the specified id.
func (s *StartTimes) Replica(id uint32) (t time.Time, ok bool) {
	t, ok = s.replicas[id]
	return
}

// ReplicaOffset returns the time offset from the replica's start time.
func (s *StartTimes) ReplicaOffset(id uint32, t time.Time) (offset time.Duration, ok bool) {
	startTime, ok := s.replicas[id]
	if !ok {
		return 0, false
	}
	return t.Sub(startTime), true
}
