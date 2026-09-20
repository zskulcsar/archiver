package linux

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// * [x] **P1_LINUX_001** Tool discovery prefers configured executable paths
// - Description: Discovers all required Linux image tools with an explicit GnuPG path and PATH lookups for PAR2 and xorriso.
// - Expected: Discovery records resolved paths and versions without invoking a shell.
func TestDiscoverTools_PrefersConfiguredPath(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{outputs: map[string]string{
		"/opt/gpg --version":        "gpg (GnuPG) 2.4.7",
		"/usr/bin/par2 --version":   "par2cmdline version 0.8.1",
		"/usr/bin/xorriso -version": "xorriso 1.5.6",
	}}
	lookup := func(name string) (string, error) { return filepath.Join("/usr/bin", name), nil }
	tools, err := DiscoverTools(context.Background(), ToolPaths{GPG: "/opt/gpg"}, lookup, runner)
	if err != nil {
		t.Fatalf("DiscoverTools() error = %v", err)
	}
	if got, want := tools.GPG.Path, "/opt/gpg"; got != want {
		t.Fatalf("gpg path = %q, want %q", got, want)
	}
	if got, want := tools.GPG.Version, "2.4.7"; got != want {
		t.Fatalf("gpg version = %q, want %q", got, want)
	}
}

// * [x] **P1_LINUX_002** Tool discovery reports every unavailable requirement
// - Description: Attempts discovery when the PATH cannot resolve any image-only dependency.
// - Expected: The returned error names all missing tools and gives installation guidance.
func TestDiscoverTools_ReportsMissingTools(t *testing.T) {
	t.Parallel()

	lookup := func(string) (string, error) { return "", errors.New("not found") }
	_, err := DiscoverTools(context.Background(), ToolPaths{}, lookup, &fakeRunner{})
	if err == nil {
		t.Fatal("DiscoverTools() error = nil, want missing dependency error")
	}
	for _, name := range []string{"gpg", "par2", "xorriso", "install"} {
		if !strings.Contains(err.Error(), name) {
			t.Fatalf("DiscoverTools() error = %q, want %q", err, name)
		}
	}
}

// * [x] **P1_LINUX_010** Capability discovery retains available tools when others are missing
// - Description: Discovers GnuPG successfully while every other required executable is absent.
// - Expected: The result identifies GnuPG as available and reports each missing capability independently.
func TestDiscoverToolStatuses_ReportsPartialAvailability(t *testing.T) {
	t.Parallel()

	lookup := func(name string) (string, error) {
		if name == "gpg" {
			return "/usr/bin/gpg", nil
		}
		return "", errors.New("not found")
	}
	runner := &fakeRunner{outputs: map[string]string{"/usr/bin/gpg --version": "gpg (GnuPG) 2.4.7"}}
	statuses := DiscoverToolStatuses(context.Background(), ToolPaths{}, lookup, runner)
	if !statuses["gpg"].Available {
		t.Fatal("gpg availability = false, want true")
	}
	if statuses["par2"].Available {
		t.Fatal("par2 availability = true, want false")
	}
}

type fakeRunner struct {
	outputs map[string]string
}

func (f *fakeRunner) Run(_ context.Context, path string, args ...string) ([]byte, error) {
	call := path + " " + strings.Join(args, " ")
	output, ok := f.outputs[call]
	if !ok {
		return nil, errors.New("unexpected command")
	}
	return []byte(output), nil
}
