package app

// VerificationError reports a failed archive, parity, or image verification.
type VerificationError struct {
	Err error
}

// Error returns the verification failure message.
func (e VerificationError) Error() string {
	return e.Err.Error()
}

// Unwrap returns the underlying verification failure.
func (e VerificationError) Unwrap() error {
	return e.Err
}

// ExitCode returns the documented verification failure exit code.
func (VerificationError) ExitCode() int {
	return 5
}
