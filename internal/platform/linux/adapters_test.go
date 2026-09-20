package linux

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/zskulcsar/archiver/internal/adapters"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
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

// * [x] **OBSERVABILITY_002** PAR2 repair test records its major phases
// - Description: Runs a successful disposable repair through a test PAR2 executable with an in-memory span recorder.
// - Expected: The trace distinguishes workspace copying, PAR2 repair, and repaired-file comparison.
func TestParityRepair_RecordsMajorPhaseSpans(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previousProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		_ = provider.Shutdown(context.Background())
	})

	directory := t.TempDir()
	payloadPath := filepath.Join(directory, "payload.enc")
	payload := []byte("original-payload")
	if err := os.WriteFile(payloadPath, payload, 0o600); err != nil {
		t.Fatalf("WriteFile() payload error = %v", err)
	}
	parityPath := filepath.Join(directory, "payload.enc.par2")
	if err := os.WriteFile(parityPath, []byte("parity"), 0o600); err != nil {
		t.Fatalf("WriteFile() parity error = %v", err)
	}
	par2Path := filepath.Join(directory, "par2")
	if err := os.WriteFile(par2Path, []byte("#!/bin/sh\nprintf 'original-payload' > \"$2\"\n"), 0o700); err != nil {
		t.Fatalf("WriteFile() PAR2 executable error = %v", err)
	}

	backends := NewBackends(Tools{PAR2: Tool{Path: par2Path}}, "", 10)
	err := backends.TestParityRepair(context.Background(), adapters.Artifact{Path: payloadPath, Size: int64(len(payload))}, []adapters.Artifact{{Path: parityPath, Size: 6}})
	if err != nil {
		t.Fatalf("TestParityRepair() error = %v", err)
	}

	want := map[string]bool{
		"archiver.par2.repair_test.workspace_copy": false,
		"archiver.par2.repair_test.repair":         false,
		"archiver.par2.repair_test.compare":        false,
	}
	for _, span := range recorder.Ended() {
		if _, ok := want[span.Name()]; ok {
			want[span.Name()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Fatalf("trace does not include %s", name)
		}
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
