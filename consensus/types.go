package consensus

import (
	"github.com/relab/hotstuff"
)

// IDSet implements a set of replica IDs. It is used to show which replicas participated in some event.
type IDSet interface {
	// Add adds an ID to the set.
	Add(id hotstuff.ID)
	// Contains returns true if the set contains the ID.
	Contains(id hotstuff.ID) bool
	// ForEach calls f for each ID in the set.
	ForEach(f func(hotstuff.ID))
	// RangeWhile calls f for each ID in the set until f returns false.
	RangeWhile(f func(hotstuff.ID) bool)
	// Len returns the number of entries in the set.
	Len() int
}

// idSetMap implements IDSet using a map.
type idSetMap map[hotstuff.ID]struct{}

// NewIDSet returns a new IDSet using the default implementation.
func NewIDSet() IDSet {
	return make(idSetMap)
}

// Add adds an ID to the set.
func (s idSetMap) Add(id hotstuff.ID) {
	s[id] = struct{}{}
}

// Contains returns true if the set contains the given ID.
func (s idSetMap) Contains(id hotstuff.ID) bool {
	_, ok := s[id]
	return ok
}

// ForEach calls f for each ID in the set.
func (s idSetMap) ForEach(f func(hotstuff.ID)) {
	for id := range s {
		f(id)
	}
}

// RangeWhile calls f for each ID in the set until f returns false.
func (s idSetMap) RangeWhile(f func(hotstuff.ID) bool) {
	for id := range s {
		if !f(id) {
			break
		}
	}
}

// Len returns the number of entries in the set.
func (s idSetMap) Len() int {
	return len(s)
}
