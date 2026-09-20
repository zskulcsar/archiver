package linux

import (
	"testing"

	"github.com/zskulcsar/archiver/internal/adapters"
)

// * [x] **P1_LINUX_003** Encryption command keeps the passphrase out of arguments
// - Description: Builds a GnuPG symmetric-encryption command using a dedicated passphrase file descriptor.
// - Expected: The command uses AES-256 and file descriptor 3 without including the passphrase value.
func TestGPGArguments_UsePassphraseFileDescriptor(t *testing.T) {
	t.Parallel()

	arguments := gpgArguments("/tmp/payload.enc")
	if got, want := arguments, []string{"--batch", "--yes", "--pinentry-mode", "loopback", "--passphrase-fd", "3", "--symmetric", "--cipher-algo", "AES256", "--output", "/tmp/payload.enc"}; !sameStrings(got, want) {
		t.Fatalf("gpg arguments = %#v, want %#v", got, want)
	}
}

// * [x] **P1_LINUX_005** Linux backends satisfy portable creation contracts
// - Description: Constructs a backend set from discovered tool identities and an owned passphrase file.
// - Expected: The result is assignable to all portable create interfaces without accessing external tools.
func TestNewBackends_ImplementsCreateContracts(t *testing.T) {
	t.Parallel()

	backends := NewBackends(Tools{GPG: Tool{Path: "gpg"}, PAR2: Tool{Path: "par2"}, Xorriso: Tool{Path: "xorriso"}}, "/tmp/passphrase", 10)
	var _ adapters.ArchiveCreator = backends
	var _ adapters.Encryptor = backends
	var _ adapters.ParityCreator = backends
	var _ adapters.ImageCreator = backends
}

// * [x] **P1_LINUX_012** PAR2 command uses the configured loss-tolerance policy
// - Description: Builds a PAR2 create command for a twenty-percent local recovery policy.
// - Expected: The command receives the requested percentage rather than a hard-coded default.
func TestPAR2Arguments_UseConfiguredLossTolerance(t *testing.T) {
	t.Parallel()

	arguments := par2CreateArguments(20, "/tmp/payload.enc.par2", "/tmp/payload.enc")
	if !containsArgument(arguments, "-r20") {
		t.Fatalf("PAR2 arguments = %#v, want configured loss tolerance", arguments)
	}
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func containsArgument(arguments []string, wanted string) bool {
	for _, argument := range arguments {
		if argument == wanted {
			return true
		}
	}
	return false
}
