package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zskulcsar/archiver/internal/domain"
)

// Config contains user-provided portable archive configuration.
type Config struct {
	Capacity         string
	Output           string
	ArchiveName      string
	ManifestPath     string
	SourcePaths      []string
	LossTolerance    string
	MinimumSplitSize string
	ExternalSymlink  string
	PassphraseFile   string
	EventsFile       string
	Now              time.Time
}

// ValidatedConfig is a normalized configuration safe for planning.
type ValidatedConfig struct {
	Config
	CapacityBytes         int64
	LossPercent           int
	MinSplitSize          int64
	RequestedMinSplitSize string
	SetID                 string
	StagingPath           string
	FinalPath             string
}

// DiskSpaceChecker reports available filesystem capacity at a path.
type DiskSpaceChecker interface {
	AvailableBytes(path string) (int64, error)
}

// ValidateConfig validates portable command configuration before output is modified.
func ValidateConfig(input Config) (ValidatedConfig, error) {
	if (len(input.SourcePaths) == 0) == (input.ManifestPath == "") {
		return ValidatedConfig{}, fmt.Errorf("provide positional source paths or --manifest, but not both")
	}
	if input.Output == "" {
		return ValidatedConfig{}, fmt.Errorf("output directory is required")
	}
	info, err := os.Stat(input.Output)
	if err != nil {
		return ValidatedConfig{}, fmt.Errorf("stat output directory: %w", err)
	}
	if !info.IsDir() {
		return ValidatedConfig{}, fmt.Errorf("output path %q is not a directory", input.Output)
	}

	capacity, err := domain.ParseCapacity(input.Capacity)
	if err != nil {
		return ValidatedConfig{}, err
	}
	lossInput := input.LossTolerance
	if lossInput == "" {
		lossInput = "10%"
	}
	lossPercent, err := domain.ParseLossTolerance(lossInput)
	if err != nil {
		return ValidatedConfig{}, err
	}
	minimumSplitInput := input.MinimumSplitSize
	if minimumSplitInput == "" {
		minimumSplitInput = "0MB"
	}
	minimumSplitSize, err := domain.ParseMinimumSplitSize(minimumSplitInput, capacity)
	if err != nil {
		return ValidatedConfig{}, err
	}
	if input.ExternalSymlink != "" && input.ExternalSymlink != "materialize" {
		return ValidatedConfig{}, fmt.Errorf("invalid external symlink policy %q", input.ExternalSymlink)
	}

	name := input.ArchiveName
	if input.ManifestPath != "" {
		if input.ArchiveName != "" {
			return ValidatedConfig{}, fmt.Errorf("--archive-name cannot be used with --manifest")
		}
		name = strings.TrimSuffix(filepath.Base(input.ManifestPath), filepath.Ext(input.ManifestPath))
	} else if !safePathComponent(name) {
		return ValidatedConfig{}, fmt.Errorf("--archive-name must be a safe single path component")
	}
	if !safePathComponent(name) {
		return ValidatedConfig{}, fmt.Errorf("archive set name must be a safe single path component")
	}

	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	setID := name + "_" + now.Format("2006-01-02_15-04")
	output := filepath.Clean(input.Output)
	return ValidatedConfig{
		Config:                input,
		CapacityBytes:         capacity,
		LossPercent:           lossPercent,
		MinSplitSize:          minimumSplitSize,
		RequestedMinSplitSize: minimumSplitInput,
		SetID:                 setID,
		StagingPath:           filepath.Join(output, ".archiver-staging-"+setID),
		FinalPath:             filepath.Join(output, setID),
	}, nil
}

// PreflightOutput confirms that publication and event paths will not overwrite existing data.
func PreflightOutput(config ValidatedConfig) error {
	for _, path := range []string{config.StagingPath, config.FinalPath} {
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("stopped before creating archive artifacts: output path %q already exists; remove or rename the existing path, or choose a different --output or --archive-name", path)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("stopped before creating archive artifacts: cannot inspect output path %q: %w; correct the path or its permissions and try again", path, err)
		}
	}
	if config.EventsFile == "" {
		return nil
	}
	eventsPath := filepath.Clean(config.EventsFile)
	if isWithin(eventsPath, config.StagingPath) || isWithin(eventsPath, config.FinalPath) {
		return fmt.Errorf("stopped before creating archive artifacts: events file %q is inside the archive output; choose an --events-file path outside the staging and final archive-set directories", eventsPath)
	}
	if _, err := os.Lstat(eventsPath); err == nil {
		return fmt.Errorf("stopped before creating archive artifacts: events file %q already exists; remove or rename the existing file, or choose a new --events-file path", eventsPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stopped before creating archive artifacts: cannot inspect events file %q: %w; correct the path or its permissions and try again", eventsPath, err)
	}

	return nil
}

// PreflightCapacity confirms the output filesystem can hold final images and one temporary disc workspace.
func PreflightCapacity(config ValidatedConfig, plan domain.ArchivePlan, checker DiskSpaceChecker) error {
	if checker == nil {
		checker = defaultDiskSpaceChecker{}
	}
	if config.CapacityBytes <= 0 {
		return fmt.Errorf("image capacity must be positive")
	}
	required := int64(len(plan.Discs) + 1)
	if required > int64(^uint64(0)>>1)/config.CapacityBytes {
		return fmt.Errorf("required output capacity exceeds supported byte range")
	}
	required *= config.CapacityBytes
	available, err := checker.AvailableBytes(config.Output)
	if err != nil {
		return fmt.Errorf("inspect available output space: %w", err)
	}
	if available < required {
		return fmt.Errorf("insufficient output space: %d bytes available, %d bytes required for final images and temporary workspace", available, required)
	}
	return nil
}

func isWithin(path, directory string) bool {
	relative, err := filepath.Rel(directory, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func safePathComponent(value string) bool {
	return value != "" && value != "." && value != ".." && filepath.Base(value) == value && !strings.ContainsAny(value, `/\\`)
}
