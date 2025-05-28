package cert

import "reflect"

type typeError struct {
	want reflect.Type
	got  reflect.Type
}

func newTypeError() error {
	return &typeError{}
}

func (err *typeError) Error() string {
	return ""
}

var _ error = (*typeError)(nil)

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
