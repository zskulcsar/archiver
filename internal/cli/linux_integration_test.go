//go:build linux

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// * [x] **P1_LINUX_014** Linux image-only creation uses installed system tools
// - Description: Creates encrypted images from one synthetic source that exceeds the usable capacity of a single image through the default Linux CLI composition.
// - Expected: Creation succeeds with all installed dependencies, publishes multiple verified images, and archives split source ranges.
func TestLinuxImageOnlyCreateIntegration(t *testing.T) {
	if os.Getenv("ARCHIVER_INTEGRATION") != "1" {
		t.Skip("set ARCHIVER_INTEGRATION=1 to run installed-tool integration tests")
	}
	for _, path := range []string{"gpg", "par2", "xorriso"} {
		if _, err := os.Stat(filepath.Join("/usr/bin", path)); err != nil {
			t.Skipf("%s is not installed", path)
		}
	}

	root := t.TempDir()
	source := filepath.Join(root, "source.txt")
	content := make([]byte, 40_000_000)
	for index := range content {
		content[index] = byte(index)
	}
	if err := os.WriteFile(source, content, 0o600); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}
	passphrase := filepath.Join(root, "passphrase")
	if err := os.WriteFile(passphrase, []byte("integration test passphrase\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(passphrase) error = %v", err)
	}
	output := filepath.Join(root, "output")
	if err := os.Mkdir(output, 0o700); err != nil {
		t.Fatalf("Mkdir(output) error = %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Execute([]string{
		"create", "--capacity", "100MB", "--output", output, "--archive-name", "fixture", "--passphrase-file", passphrase, source,
	}, &stdout, &stderr, BuildInfo{Version: "test"})
	if exitCode != 0 {
		t.Fatalf("Execute() exit code = %d, want 0; stderr = %q", exitCode, stderr.String())
	}
	matches, err := filepath.Glob(filepath.Join(output, "fixture_*", "*.img"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("published images = %#v, want two images", matches)
	}
	archiveSet := filepath.Dir(matches[0])
	stdout.Reset()
	stderr.Reset()
	exitCode = Execute([]string{"verify", archiveSet}, &stdout, &stderr, BuildInfo{Version: "test"})
	if exitCode != 0 {
		t.Fatalf("verify exit code = %d, want 0; stderr = %q", exitCode, stderr.String())
	}
}
