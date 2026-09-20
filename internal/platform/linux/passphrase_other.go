//go:build !linux

package linux

import "fmt"

// ValidatePassphraseFile is unavailable outside Linux until the platform access-control policy is implemented.
func ValidatePassphraseFile(string) error {
	return fmt.Errorf("passphrase-file validation is only implemented on Linux")
}
