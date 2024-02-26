package crypto

import (
	"reflect"

	"github.com/relab/hotstuff"
	"golang.org/x/exp/slices"
)

// Signature is the individual component in MultiSignature
type Signature interface {
	Signer() hotstuff.ID
	ToBytes() []byte
}

// MultiSignature is a set of (partial) signatures.
type MultiSignature[T Signature] map[hotstuff.ID]T

// RestoreMultiSignature should only be used to restore an existing threshold signature from a set of signatures.
func RestoreMultiSignature[T Signature](signatures []T) MultiSignature[T] {
	sig := make(MultiSignature[T], len(signatures))
	for _, s := range signatures {
		sig[s.Signer()] = s
	}
	return sig
}

// ToBytes returns the object as bytes.
func (sig MultiSignature[T]) ToBytes() []byte {
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
func (sig MultiSignature[T]) Participants() hotstuff.IDSet {
	return sig
}

// Add adds an ID to the set.
func (sig MultiSignature[T]) Add(_ hotstuff.ID) {
	panic("not implemented")
}

// Contains returns true if the set contains the ID.
func (sig MultiSignature[T]) Contains(id hotstuff.ID) bool {
	_, ok := sig[id]
	return ok
}

// ForEach calls f for each ID in the set.
func (sig MultiSignature[T]) ForEach(f func(hotstuff.ID)) {
	for id := range sig {
		f(id)
	}
}

// RangeWhile calls f for each ID in the set until f returns false.
func (sig MultiSignature[T]) RangeWhile(f func(hotstuff.ID) bool) {
	for id := range sig {
		if !f(id) {
			break
		}
	}
}

// Len returns the number of entries in the set.
func (sig MultiSignature[T]) Len() int {
	return len(sig)
}

func (sig MultiSignature[T]) String() string {
	return hotstuff.IDSetToString(sig)
}

func (sig MultiSignature[T]) Type() reflect.Type {
	for _, s := range sig {
		return reflect.TypeOf(s)
	}
	return nil
}
