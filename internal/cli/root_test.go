package cli

import (
	"bytes"
	"strings"
	"testing"
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
