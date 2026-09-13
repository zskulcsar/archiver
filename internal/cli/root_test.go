package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zskulcsar/archiver/internal/adapters"
	"github.com/zskulcsar/archiver/internal/app"
	"github.com/zskulcsar/archiver/internal/domain"
)

// * [x] **P1_CLI_001** Cobra help command output
// - Description: Runs the Cobra root command with the help flag through the CLI adapter.
// - Expected: It writes Cobra usage information to standard output and exits successfully.
func TestExecute_Help(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Execute([]string{"--help"}, &stdout, &stderr, BuildInfo{Version: "dev"})

	if exitCode != 0 {
		t.Fatalf("Execute() exit code = %d, want 0", exitCode)
	}

	if got, want := stdout.String(), "archiver [command]"; !strings.Contains(got, want) {
		t.Fatalf("Execute() stdout = %q, want to contain %q", got, want)
	}

	if got := stderr.String(); got != "" {
		t.Fatalf("Execute() stderr = %q, want empty", got)
	}
}

// * [x] **P1_CLI_002** Cobra version command output
// - Description: Runs the Cobra version subcommand with an injected build identity.
// - Expected: It writes the supplied version and revision to standard output and exits successfully.
func TestExecute_Version(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Execute([]string{"version"}, &stdout, &stderr, BuildInfo{
		Version:  "v0.0.0",
		Revision: "abc123",
	})

	if exitCode != 0 {
		t.Fatalf("Execute() exit code = %d, want 0", exitCode)
	}

	if got, want := stdout.String(), "archiver v0.0.0 (abc123)\n"; got != want {
		t.Fatalf("Execute() stdout = %q, want %q", got, want)
	}

	if got := stderr.String(); got != "" {
		t.Fatalf("Execute() stderr = %q, want empty", got)
	}
}

// * [x] **P1_CLI_003** Cobra unknown command handling
// - Description: Runs the Cobra root command with an unsupported command.
// - Expected: It writes a diagnostic to standard error and exits with code 2.
func TestExecute_UnknownCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Execute([]string{"unknown"}, &stdout, &stderr, BuildInfo{Version: "dev"})

	if exitCode != 2 {
		t.Fatalf("Execute() exit code = %d, want 2", exitCode)
	}

	if got := stdout.String(); got != "" {
		t.Fatalf("Execute() stdout = %q, want empty", got)
	}

	if got, want := stderr.String(), "unknown command \"unknown\" for \"archiver\"\n"; got != want {
		t.Fatalf("Execute() stderr = %q, want %q", got, want)
	}
}

// * [x] **P1_CLI_004** Cobra plan command invokes portable planning service
// - Description: Supplies valid source-style configuration to the plan command through an injected portable service.
// - Expected: The service receives validated configuration, a human-readable plan is printed, and the command exits successfully.
func TestExecuteWithServices_Plan(t *testing.T) {
	t.Parallel()

	output := t.TempDir()
	services := fakeServices{plan: func(_ context.Context, config app.ValidatedConfig) (domain.ArchivePlan, error) {
		if got, want := config.ArchiveName, "photos"; got != want {
			return domain.ArchivePlan{}, fmt.Errorf("archive name = %q, want %q", got, want)
		}
		return domain.ArchivePlan{Discs: []domain.Disc{{Number: 1}}}, nil
	}}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := ExecuteWithServices([]string{"plan", "--capacity", "25GB", "--output", output, "--archive-name", "photos", "/input/photos"}, &stdout, &stderr, BuildInfo{Version: "dev"}, services)

	if exitCode != 0 {
		t.Fatalf("ExecuteWithServices() exit code = %d, want 0; stderr = %q", exitCode, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Plan: 1 disc(s)\nPlan written to ") {
		t.Fatalf("ExecuteWithServices() stdout = %q, want plan file notice", got)
	}
}

// * [x] **P1_CLI_007** Default positional-source planning
// - Description: Runs the default CLI service composition against a real local source file.
// - Expected: The command collects the source and prints its archive allocation plan without external tools.
func TestExecute_PlanWithPositionalSource(t *testing.T) {
	source := filepath.Join(t.TempDir(), "photo.jpg")
	if err := os.WriteFile(source, []byte("photo"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Execute([]string{"plan", "--capacity", "25GB", "--output", t.TempDir(), "--archive-name", "photos", source}, &stdout, &stderr, BuildInfo{Version: "dev"})

	if exitCode != 0 {
		t.Fatalf("Execute() exit code = %d, want 0; stderr = %q", exitCode, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Plan: 1 disc(s)\nPlan written to ") {
		t.Fatalf("Execute() stdout = %q, want plan file notice", got)
	}
}

// * [x] **P1_CLI_008** Default manifest planning
// - Description: Runs the default CLI service composition against a real SQLite source manifest.
// - Expected: The command reads manifest sources and prints their archive allocation plan without external tools.
func TestExecute_PlanWithManifest(t *testing.T) {
	manifestPath := filepath.Join(t.TempDir(), "sources.sqlite")
	manifest, err := adapters.OpenManifest(manifestPath)
	if err != nil {
		t.Fatalf("OpenManifest() error = %v", err)
	}
	t.Cleanup(func() {
		if err := manifest.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	if err := manifest.ReplaceSources(context.Background(), []domain.Source{{SourcePath: "/sources/photo.jpg", LogicalPath: "photo.jpg", Size: 5}}); err != nil {
		t.Fatalf("ReplaceSources() error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Execute([]string{"plan", "--capacity", "25GB", "--output", t.TempDir(), "--manifest", manifestPath}, &stdout, &stderr, BuildInfo{Version: "dev"})

	if exitCode != 0 {
		t.Fatalf("Execute() exit code = %d, want 0; stderr = %q", exitCode, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Plan: 1 disc(s)\nPlan written to ") {
		t.Fatalf("Execute() stdout = %q, want plan file notice", got)
	}
}

// * [x] **P1_CLI_009** Default create source resolution before dependency error
// - Description: Runs create with a missing positional source through the default CLI service composition.
// - Expected: Source validation fails instead of reporting unavailable archive tools before planning can occur.
func TestExecute_CreateValidatesSourcesBeforeMissingBackend(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Execute([]string{"create", "--capacity", "25GB", "--output", t.TempDir(), "--archive-name", "photos", filepath.Join(t.TempDir(), "missing.jpg")}, &stdout, &stderr, BuildInfo{Version: "dev"})

	if exitCode != 1 {
		t.Fatalf("Execute() exit code = %d, want 1; stderr = %q", exitCode, stderr.String())
	}
	if got, want := stderr.String(), "inspect source path"; !strings.Contains(got, want) {
		t.Fatalf("Execute() stderr = %q, want to contain %q", got, want)
	}
}

// * [x] **P1_CLI_010** Default create backend dependency error
// - Description: Runs create with a valid local source through the default CLI service composition.
// - Expected: After source collection and planning, the command reports that the required archive tool backends are unavailable.
func TestExecute_CreateReportsMissingBackendsAfterPlanning(t *testing.T) {
	source := filepath.Join(t.TempDir(), "photo.jpg")
	if err := os.WriteFile(source, []byte("photo"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Execute([]string{"create", "--capacity", "25GB", "--output", t.TempDir(), "--archive-name", "photos", source}, &stdout, &stderr, BuildInfo{Version: "dev"})

	if exitCode != 4 {
		t.Fatalf("Execute() exit code = %d, want 4; stderr = %q", exitCode, stderr.String())
	}
	if got, want := stderr.String(), "required archive, encryption, parity, and image backends are unavailable\n"; got != want {
		t.Fatalf("Execute() stderr = %q, want %q", got, want)
	}
}

// * [x] **P1_CLI_005** Cobra create command writes JSONL events
// - Description: Creates an event file through the command boundary while the injected service emits a stable lifecycle event.
// - Expected: The event file contains newline-delimited JSON and the command reports successful creation.
func TestExecuteWithServices_CreateWritesEventsFile(t *testing.T) {
	t.Parallel()

	output := t.TempDir()
	eventsPath := filepath.Join(t.TempDir(), "events.jsonl")
	services := fakeServices{create: func(_ context.Context, _ app.ValidatedConfig, events app.EventSink) error {
		return events.Emit(app.Event{Type: "published"})
	}}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := ExecuteWithServices([]string{"create", "--capacity", "25GB", "--output", output, "--archive-name", "photos", "--events-file", eventsPath, "/input/photos"}, &stdout, &stderr, BuildInfo{Version: "dev"}, services)

	if exitCode != 0 {
		t.Fatalf("ExecuteWithServices() exit code = %d, want 0; stderr = %q", exitCode, stderr.String())
	}
	contents, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got, want := string(contents), "{\"type\":\"published\"}\n"; got != want {
		t.Fatalf("events file = %q, want %q", got, want)
	}
}

// * [x] **P1_CLI_006** Cobra errors use documented exit codes
// - Description: Invokes an unavailable backend and invalid archive configuration through the command adapter.
// - Expected: Missing dependencies exit 4 and invalid configuration exits 3 without a Go error classification leak.
func TestExecuteWithServices_MapsOperationalErrors(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		args     []string
		services Services
		wantCode int
	}{
		"dependency": {
			args:     []string{"create", "--capacity", "25GB", "--output", t.TempDir(), "--archive-name", "photos", "/input/photos"},
			services: unavailableServices(),
			wantCode: 4,
		},
		"configuration": {
			args:     []string{"plan", "--capacity", "bad", "--output", t.TempDir(), "--archive-name", "photos", "/input/photos"},
			services: fakeServices{},
			wantCode: 3,
		},
		"verification": {
			args: []string{"verify", "/archive-set"},
			services: fakeServices{verify: func(context.Context, string, app.EventSink) error {
				return app.VerificationError{Err: errors.New("image checksum mismatch")}
			}},
			wantCode: 5,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer

			exitCode := ExecuteWithServices(tc.args, &stdout, &stderr, BuildInfo{Version: "dev"}, tc.services)

			if exitCode != tc.wantCode {
				t.Fatalf("ExecuteWithServices() exit code = %d, want %d; stderr = %q", exitCode, tc.wantCode, stderr.String())
			}
		})
	}
}

type fakeServices struct {
	plan   func(context.Context, app.ValidatedConfig) (domain.ArchivePlan, error)
	create func(context.Context, app.ValidatedConfig, app.EventSink) error
	verify func(context.Context, string, app.EventSink) error
}

func (f fakeServices) Backends(context.Context) ([]Backend, error) {
	return []Backend{{Name: "fake", Available: true}}, nil
}

func (f fakeServices) Plan(ctx context.Context, config app.ValidatedConfig) (domain.ArchivePlan, error) {
	if f.plan == nil {
		return domain.ArchivePlan{}, nil
	}
	return f.plan(ctx, config)
}

func (f fakeServices) Create(ctx context.Context, config app.ValidatedConfig, events app.EventSink) error {
	if f.create == nil {
		return nil
	}
	return f.create(ctx, config, events)
}

func (f fakeServices) Verify(ctx context.Context, archiveSet string, events app.EventSink) error {
	if f.verify == nil {
		return nil
	}
	return f.verify(ctx, archiveSet, events)
}
