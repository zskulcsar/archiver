//go:build !linux

package linux

import "fmt"

// Classifier is unavailable outside Linux.
type Classifier struct{}

// IsLocal reports that Linux filesystem classification is unavailable.
func (Classifier) IsLocal(string) (bool, error) {
	return false, fmt.Errorf("local filesystem classification is only supported on linux")
}
