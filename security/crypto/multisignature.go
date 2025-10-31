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
type Multi[T Signature] []T

// NewMulti creates a new Multi from the given signatures.
// The provided signatures are assumed to be sorted by signer ID.
func NewMulti[T Signature](sigs ...T) Multi[T] {
	return Multi[T](sigs)
}

// NewMultiSorted creates a new Multi from the given signatures
// ensuring they are sorted by signer ID.
func NewMultiSorted[T Signature](sigs ...T) Multi[T] {
	slices.SortFunc(sigs, func(a, b T) int { return int(a.Signer()) - int(b.Signer()) })
	return Multi[T](sigs)
}

// ToBytes returns the object as bytes.
func (sig Multi[T]) ToBytes() []byte {
	var b []byte
	for _, signature := range sig {
		b = append(b, signature.ToBytes()...)
	}
	return b
}

// Participants returns the IDs of replicas who participated in the threshold signature.
func (sig Multi[T]) Participants() hotstuff.IDSet {
	return sig
}

// Add is not supported for Multi.
func (sig Multi[T]) Add(_ hotstuff.ID) {
	panic("not implemented")
}

// Contains returns true if the set contains the ID.
func (sig Multi[T]) Contains(id hotstuff.ID) bool {
	return slices.ContainsFunc(sig, func(s T) bool {
		return s.Signer() == id
	})
}

// ForEach calls f for each ID in the set.
func (sig Multi[T]) ForEach(f func(hotstuff.ID)) {
	for _, s := range sig {
		f(s.Signer())
	}
}

// RangeWhile calls f for each ID in the set until f returns false.
func (sig Multi[T]) RangeWhile(f func(hotstuff.ID) bool) {
	for _, s := range sig {
		if !f(s.Signer()) {
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

var (
	_ hotstuff.QuorumSignature = (*Multi[Signature])(nil)
	_ hotstuff.IDSet           = (*Multi[Signature])(nil)
)
