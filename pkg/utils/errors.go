package utils

import "errors"

// MaxErrorString checks if the string representation of provided error is
// longer than given length and if so it will create new error message with
// max provided length. This function will change the type of the error and
// should be used only when printing potentially long messages.
func MaxErrorString(err error, maxLength int) error {
	errorStr := err.Error()
	if len(errorStr) > maxLength {
		return errors.New(errorStr[:maxLength])
	}
	return err
}
