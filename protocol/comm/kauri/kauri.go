// Package kauri contains the utilities for the Kauri protocol
package kauri

import (
	"errors"
	"slices"

	"github.com/relab/hotstuff"
)

// CanMergeContributions returns nil if the contributions are non-overlapping.
func CanMergeContributions(a, b hotstuff.QuorumSignature) error {
	if a == nil || b == nil {
		return errors.New("cannot merge nil contributions")
	}
	canMerge := true
	a.Participants().RangeWhile(func(i hotstuff.ID) bool {
		// cannot merge a and b if both contain a contribution from the same ID.
		canMerge = !b.Participants().Contains(i)
		return canMerge // exit the range-while loop if canMerge is false
	})
	if !canMerge {
		return errors.New("cannot merge overlapping contributions")
	}
	return nil
}

// IsSubSet returns true if all elements in a are contained in b.
func IsSubSet(a, b []hotstuff.ID) bool {
	for _, id := range a {
		if !slices.Contains(b, id) {
			return false
		}
	}
	return true
}
