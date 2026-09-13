package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zskulcsar/archiver/internal/domain"
)

type diskSpaceFunc func(string) (int64, error)

func (f diskSpaceFunc) AvailableBytes(path string) (int64, error) {
	return f(path)
}

// * [x] **P1_CORE_007** Source configuration derives a safe set ID
// - Description: Validates positional source input and combines the supplied archive name with the execution timestamp.
// - Expected: The resulting set ID and staging paths are deterministic and beneath the output parent.
func TestValidateConfig_SourceInput(t *testing.T) {
	t.Parallel()

	output := t.TempDir()
	config, err := ValidateConfig(Config{
		Capacity:      "25GB",
		Output:        output,
		ArchiveName:   "family-photos",
		SourcePaths:   []string{"/input/photos"},
		LossTolerance: "10%",
		Now:           time.Date(2026, 9, 13, 14, 5, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}

	if got, want := config.SetID, "family-photos_2026-09-13_14-05"; got != want {
		t.Fatalf("SetID = %q, want %q", got, want)
	}
	if got, want := config.MinSplitSize, int64(0); got != want {
		t.Fatalf("MinSplitSize = %d, want %d", got, want)
	}
	if got, want := config.StagingPath, filepath.Join(output, ".archiver-staging-"+config.SetID); got != want {
		t.Fatalf("StagingPath = %q, want %q", got, want)
	}
}

// * [x] **P1_CORE_009** Output preflight rejects existing publication paths
// - Description: Detects a prior staging directory before archive creation can overwrite it.
// - Expected: Preflight fails and leaves the output directory unchanged.
func TestPreflightOutput_RejectsExistingStagingDirectory(t *testing.T) {
	t.Parallel()

	output := t.TempDir()
	config, err := ValidateConfig(Config{
		Capacity:    "25GB",
		Output:      output,
		ArchiveName: "backup",
		SourcePaths: []string{"/input"},
		Now:         time.Date(2026, 9, 13, 14, 5, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}
	if err := os.Mkdir(config.StagingPath, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	err = PreflightOutput(config)
	if err == nil {
		t.Fatal("PreflightOutput() error = nil, want error")
	}
	for _, want := range []string{"stopped before creating archive artifacts", "already exists", "remove or rename"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("PreflightOutput() error = %q, want to contain %q", err, want)
		}
	}
}

// * [x] **P1_CORE_016** Output capacity preflight
// - Description: Requires enough free space for every planned final image and one additional disc of temporary workspace.
// - Expected: Preflight rejects insufficient capacity and propagates disk-space inspection failures.
func TestPreflightCapacity_RequiresFinalImagesAndWorkspace(t *testing.T) {
	t.Parallel()

	config := testValidatedConfig(t.TempDir())
	config.CapacityBytes = 100
	plan := domain.ArchivePlan{Discs: []domain.Disc{{Number: 1}, {Number: 2}}}

	err := PreflightCapacity(config, plan, diskSpaceFunc(func(string) (int64, error) {
		return 299, nil
	}))
	if err == nil {
		t.Fatal("PreflightCapacity() error = nil, want insufficient-space error")
	}

	err = PreflightCapacity(config, plan, diskSpaceFunc(func(string) (int64, error) {
		return 0, errors.New("unavailable")
	}))
	if err == nil {
		t.Fatal("PreflightCapacity() error = nil, want disk-space checker error")
	}

	if err := PreflightCapacity(config, plan, diskSpaceFunc(func(string) (int64, error) {
		return 300, nil
	})); err != nil {
		t.Fatalf("PreflightCapacity() error = %v", err)
	}
}

// * [x] **P1_CORE_008** Exclusive source-selection mechanisms
// - Description: Rejects a request that supplies both positional source paths and a manifest.
// - Expected: Validation fails before any output operation is attempted.
func TestValidateConfig_RejectsMixedSourceSelection(t *testing.T) {
	t.Parallel()

	_, err := ValidateConfig(Config{
		Capacity:     "25GB",
		Output:       t.TempDir(),
		ManifestPath: "manifest.sqlite",
		SourcePaths:  []string{"/input/photos"},
		Now:          time.Now(),
	})
	if err == nil {
		t.Fatal("ValidateConfig() error = nil, want error")
	}
}
