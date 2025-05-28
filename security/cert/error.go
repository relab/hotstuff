package cert

type FatalError struct {
	message string
}

func newFatalError(message string) error {
	return &FatalError{
		message: message,
	}
}

func (err *FatalError) Error() string {
	return err.message
}

var _ error = (*FatalError)(nil)
