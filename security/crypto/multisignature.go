package crypto

import (
	"slices"

	"github.com/relab/hotstuff"
)

// Signature is the individual component in MultiSignature
type Signature interface {
	Signer() hotstuff.ID
	ToBytes() []byte
}

// Multi is a set of (partial) signatures.
type Multi[T Signature] map[hotstuff.ID]T

// Restore should only be used to restore an existing threshold signature from a set of signatures.
func Restore[T Signature](signatures []T) Multi[T] {
	sig := make(Multi[T], len(signatures))
	for _, s := range signatures {
		sig[s.Signer()] = s
	}
	return sig
}

// ToBytes returns the object as bytes.
func (sig Multi[T]) ToBytes() []byte {
	var b []byte
	// sort by ID to make it deterministic
	order := make([]hotstuff.ID, 0, len(sig))
	for _, signature := range sig {
		order = append(order, signature.Signer())
	}
	slices.Sort(order)
	for _, id := range order {
		b = append(b, sig[id].ToBytes()...)
	}
	return b
}

// Participants returns the IDs of replicas who participated in the threshold signature.
func (sig Multi[T]) Participants() hotstuff.IDSet {
	return sig
}

// Add adds an ID to the set.
func (sig Multi[T]) Add(_ hotstuff.ID) {
	panic("not implemented")
}

// Contains returns true if the set contains the ID.
func (sig Multi[T]) Contains(id hotstuff.ID) bool {
	_, ok := sig[id]
	return ok
}

// ForEach calls f for each ID in the set.
func (sig Multi[T]) ForEach(f func(hotstuff.ID)) {
	for id := range sig {
		f(id)
	}
}

// RangeWhile calls f for each ID in the set until f returns false.
func (sig Multi[T]) RangeWhile(f func(hotstuff.ID) bool) {
	for id := range sig {
		if !f(id) {
			break
		}
	}
}

// Len returns the number of entries in the set.
func (sig Multi[T]) Len() int {
	return len(sig)
}

func (sig Multi[T]) String() string {
	return hotstuff.IDSetToString(sig)
}
