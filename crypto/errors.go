package crypto

import "fmt"

var (
	// ErrHashMismatch is the error used when a partial certificate hash does not match the hash of a block.
	ErrHashMismatch = fmt.Errorf("certificate hash does not match block hash")

	// ErrPartialDuplicate is the error used when two or more signatures were created by the same replica.
	ErrPartialDuplicate = fmt.Errorf("cannot add more than one signature per replica")

	// ErrViewMismatch is the error used when timeouts have different views.
	ErrViewMismatch = fmt.Errorf("timeout views do not match")

	// ErrNotAQuorum is the error used when a quorum is not reached.
	ErrNotAQuorum = fmt.Errorf("not a quorum")

	// ErrWrongType is the error used when an incompatible type is encountered.
	ErrWrongType = fmt.Errorf("incompatible type")
)
