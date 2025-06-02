package crypto

import (
	"errors"
)

var (
	// ErrCombineMultiple is used when Combine is called with less than two signatures.
	ErrCombineMultiple = errors.New("must have at least two signatures")

	// ErrCombineOverlap is used when Combine is called with signatures that have overlapping participation.
	ErrCombineOverlap = errors.New("overlapping signatures")
)
