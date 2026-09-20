//go:build linux

package linux

import (
	"fmt"
	"os"
	"syscall"
)

// ValidatePassphraseFile confirms that path is a private regular file owned by the invoking user.
func ValidatePassphraseFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("inspect passphrase file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("passphrase file %q must be a regular file", path)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("passphrase file %q grants group or other access", path)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("inspect passphrase file ownership %q", path)
	}
	if stat.Uid != uint32(os.Geteuid()) {
		return fmt.Errorf("passphrase file %q is not owned by the invoking user", path)
	}
	return nil
}
