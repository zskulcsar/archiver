package linux

import (
	"os"
	"path/filepath"
	"testing"
)

// * [x] **P1_LINUX_006** Passphrase files require private regular-file permissions
// - Description: Validates a caller-owned passphrase file with private mode, then makes it group-readable.
// - Expected: The private regular file is accepted and the group-readable file is rejected.
func TestValidatePassphraseFile_RequiresPrivateRegularFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "passphrase")
	if err := os.WriteFile(path, []byte("secret\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := ValidatePassphraseFile(path); err != nil {
		t.Fatalf("ValidatePassphraseFile() error = %v", err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}
	if err := ValidatePassphraseFile(path); err == nil {
		t.Fatal("ValidatePassphraseFile() error = nil, want insecure mode error")
	}
}
