// Package cli adapts command-line input to Archiver application behavior.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zskulcsar/archiver/internal/adapters"
	"github.com/zskulcsar/archiver/internal/app"
	"github.com/zskulcsar/archiver/internal/domain"
	platformlinux "github.com/zskulcsar/archiver/internal/platform/linux"
)

// BuildInfo identifies the CLI build displayed by the version command.
type BuildInfo struct {
	Version  string
	Revision string
}

// Execute runs the Archiver command tree with the supplied arguments and streams.
func Execute(args []string, stdout, stderr io.Writer, buildInfo BuildInfo) int {
	return ExecuteWithServices(args, stdout, stderr, buildInfo, defaultServices())
}

// ExecuteWithServices runs the Archiver command tree with portable services.
func ExecuteWithServices(args []string, stdout, stderr io.Writer, buildInfo BuildInfo, services Services) int {
	root := newRootCommand(buildInfo, services)
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)

	if err := root.Execute(); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		if isUsageError(err) {
			return 2
		}
		return exitCode(err)
	}

	return 0
}

// NewRootCommand creates the Archiver Cobra command tree.
func NewRootCommand(buildInfo BuildInfo) *cobra.Command {
	return newRootCommand(buildInfo, defaultServices())
}

func newRootCommand(buildInfo BuildInfo, services Services) *cobra.Command {
	root := &cobra.Command{
		Use:           "archiver",
		Short:         "Create encrypted, multi-disc archive images",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	root.AddCommand(newVersionCommand(buildInfo))
	root.AddCommand(newBackendsCommand(services))
	root.AddCommand(newPlanCommand(services))
	root.AddCommand(newCreateCommand(services))
	root.AddCommand(newVerifyCommand(services))
	return root
}

func newVersionCommand(buildInfo BuildInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show the CLI version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), formatVersion(buildInfo))
			return err
		},
	}
}

func formatVersion(buildInfo BuildInfo) string {
	if buildInfo.Revision == "" {
		return fmt.Sprintf("archiver %s", buildInfo.Version)
	}

	return fmt.Sprintf("archiver %s (%s)", buildInfo.Version, buildInfo.Revision)
}

// Backend is one discovered adapter and its portable capability status.
type Backend struct {
	Name         string
	Version      string
	Available    bool
	Capabilities []string
}

// Services supplies command use cases at the CLI boundary.
type Services interface {
	Backends(context.Context) ([]Backend, error)
	Plan(context.Context, app.ValidatedConfig) (domain.ArchivePlan, error)
	Create(context.Context, app.ValidatedConfig, app.EventSink) error
	Verify(context.Context, string, app.EventSink) error
}

type unavailableService struct{}

type portableServices struct {
	filesystems app.LocalFilesystem
}

func defaultServices() Services {
	return portableServices{filesystems: platformlinux.Classifier{}}
}

func (s portableServices) Backends(context.Context) ([]Backend, error) {
	return nil, nil
}

func (s portableServices) Plan(ctx context.Context, config app.ValidatedConfig) (domain.ArchivePlan, error) {
	sources, err := s.sources(ctx, config)
	if err != nil {
		return domain.ArchivePlan{}, err
	}
	plan, err := domain.PlanWithMinimumSplitSize(sources, config.CapacityBytes, config.MinSplitSize)
	if err != nil {
		return domain.ArchivePlan{}, fmt.Errorf("plan archive: %w", err)
	}
	return plan, nil
}

func (s portableServices) Create(ctx context.Context, config app.ValidatedConfig, _ app.EventSink) error {
	if _, err := s.Plan(ctx, config); err != nil {
		return err
	}
	return dependencyError{errors.New("required archive, encryption, parity, and image backends are unavailable")}
}

func (portableServices) Verify(context.Context, string, app.EventSink) error {
	return dependencyError{errors.New("archive-set verification backend is not configured")}
}

func (s portableServices) sources(ctx context.Context, config app.ValidatedConfig) ([]domain.Source, error) {
	if config.ManifestPath != "" {
		manifest, err := adapters.OpenManifest(config.ManifestPath)
		if err != nil {
			return nil, err
		}
		defer func() { _ = manifest.Close() }()
		return manifest.Sources(ctx)
	}

	files, err := app.CollectSourceFilesWithPolicy(config.SourcePaths, s.filesystems, config.ExternalSymlink)
	if err != nil {
		return nil, err
	}
	sources := make([]domain.Source, 0, len(files))
	for _, file := range files {
		if file.SymlinkTarget != "" {
			sources = append(sources, domain.Source{LogicalPath: file.LogicalPath, SymlinkTarget: file.SymlinkTarget})
			continue
		}
		info, err := os.Stat(file.Path)
		if err != nil {
			return nil, fmt.Errorf("inspect source file %q: %w", file.Path, err)
		}
		sources = append(sources, domain.Source{
			SourcePath:  file.Path,
			LogicalPath: file.LogicalPath,
			Size:        info.Size(),
		})
	}
	return sources, nil
}

func unavailableServices() Services {
	return unavailableService{}
}

func (unavailableService) Backends(context.Context) ([]Backend, error) {
	return nil, nil
}

func (unavailableService) Plan(context.Context, app.ValidatedConfig) (domain.ArchivePlan, error) {
	return domain.ArchivePlan{}, dependencyError{errors.New("no portable source and tool backends are configured")}
}

func (unavailableService) Create(context.Context, app.ValidatedConfig, app.EventSink) error {
	return dependencyError{errors.New("no archive, encryption, parity, or image backend is configured")}
}

func (unavailableService) Verify(context.Context, string, app.EventSink) error {
	return dependencyError{errors.New("no archive-set verification backend is configured")}
}

func newBackendsCommand(services Services) *cobra.Command {
	var eventsFile string
	command := &cobra.Command{
		Use:   "backends",
		Short: "Report available archive backends",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			backends, err := services.Backends(cmd.Context())
			if err != nil {
				return err
			}
			for _, backend := range backends {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\tavailable=%t\tversion=%s\n", backend.Name, backend.Available, backend.Version); err != nil {
					return err
				}
			}
			return writeStandaloneEvent(eventsFile, app.Event{Type: "backends_completed"})
		},
	}
	command.Flags().StringVar(&eventsFile, "events-file", "", "Write JSONL events to a new file")
	return command
}

func newPlanCommand(services Services) *cobra.Command {
	config := app.Config{}
	command := &cobra.Command{
		Use:   "plan [SOURCE...]",
		Short: "Print an archive allocation plan",
		RunE: func(cmd *cobra.Command, args []string) error {
			validated, err := validateCommandConfig(config, args)
			if err != nil {
				return err
			}
			if err := app.PreflightOutput(validated); err != nil {
				return configurationError{err}
			}
			plan, err := services.Plan(cmd.Context(), validated)
			if err != nil {
				return err
			}
			planPath, err := app.WritePlanJSON(validated, plan)
			if err != nil {
				return configurationError{err}
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Plan: %d disc(s)\n", len(plan.Discs)); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Plan written to %s\n", planPath); err != nil {
				return err
			}
			return writeConfigEvent(validated, app.Event{Type: "plan_completed"})
		},
	}
	addArchiveConfigFlags(command, &config, false)
	return command
}

func newCreateCommand(services Services) *cobra.Command {
	config := app.Config{}
	command := &cobra.Command{
		Use:   "create [SOURCE...]",
		Short: "Create and verify archive images",
		RunE: func(cmd *cobra.Command, args []string) error {
			validated, err := validateCommandConfig(config, args)
			if err != nil {
				return err
			}
			if err := app.PreflightOutput(validated); err != nil {
				return configurationError{err}
			}
			events, closeEvents, err := openConfigEvents(validated)
			if err != nil {
				return configurationError{err}
			}
			defer closeEvents()
			validated.EventsFile = ""
			if err := services.Create(cmd.Context(), validated, events); err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "Archive set created")
			return err
		},
	}
	addArchiveConfigFlags(command, &config, true)
	return command
}

func newVerifyCommand(services Services) *cobra.Command {
	var eventsFile string
	command := &cobra.Command{
		Use:   "verify ARCHIVE_SET_DIR",
		Short: "Verify a published archive set",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			events, closeEvents, err := openEventsFile(eventsFile)
			if err != nil {
				return configurationError{err}
			}
			defer closeEvents()
			if err := services.Verify(cmd.Context(), args[0], events); err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "Archive set verified")
			return err
		},
	}
	command.Flags().StringVar(&eventsFile, "events-file", "", "Write JSONL events to a new file")
	return command
}

func addArchiveConfigFlags(command *cobra.Command, config *app.Config, includePassphrase bool) {
	command.Flags().StringVar(&config.Capacity, "capacity", "", "Target media capacity, such as 25GB")
	command.Flags().StringVar(&config.Output, "output", "", "Archive-set output directory")
	command.Flags().StringVar(&config.ArchiveName, "archive-name", "", "Safe archive-set name for positional sources")
	command.Flags().StringVar(&config.ManifestPath, "manifest", "", "SQLite source manifest")
	command.Flags().StringVar(&config.LossTolerance, "loss-tolerance", "", "Local recovery data percentage")
	command.Flags().StringVar(&config.MinimumSplitSize, "min-split-size", "", "Minimum split part size, such as 500MB or 5%")
	command.Flags().StringVar(&config.ExternalSymlink, "external-symlink", "", "Materialize external symlink targets (only supported value: materialize)")
	command.Flags().StringVar(&config.EventsFile, "events-file", "", "Write JSONL events to a new file")
	if includePassphrase {
		command.Flags().StringVar(&config.PassphraseFile, "passphrase-file", "", "Passphrase file")
	}
}

func validateCommandConfig(config app.Config, args []string) (app.ValidatedConfig, error) {
	config.SourcePaths = args
	config.Now = time.Now()
	validated, err := app.ValidateConfig(config)
	if err != nil {
		return app.ValidatedConfig{}, configurationError{err}
	}
	return validated, nil
}

func writeConfigEvent(config app.ValidatedConfig, event app.Event) error {
	if err := app.PreflightOutput(config); err != nil {
		return configurationError{err}
	}
	return writeStandaloneEvent(config.EventsFile, event)
}

func writeStandaloneEvent(path string, event app.Event) error {
	events, closeEvents, err := openEventsFile(path)
	if err != nil {
		return configurationError{err}
	}
	defer closeEvents()
	if events == nil {
		return nil
	}
	return events.Emit(event)
}

func openConfigEvents(config app.ValidatedConfig) (app.EventSink, func(), error) {
	return openEventsFile(config.EventsFile)
}

func openEventsFile(path string) (app.EventSink, func(), error) {
	if path == "" {
		return nil, func() {}, nil
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return nil, nil, fmt.Errorf("stopped before command execution: events file %q already exists; remove or rename the existing file, or choose a new --events-file path", path)
		}
		return nil, nil, fmt.Errorf("create events file: %w", err)
	}
	return app.NewJSONLEventWriter(file), func() { _ = file.Close() }, nil
}

type configurationError struct{ error }

func (configurationError) ExitCode() int { return 3 }

type dependencyError struct{ error }

func (dependencyError) ExitCode() int { return 4 }

type exitCoder interface {
	ExitCode() int
}

func exitCode(err error) int {
	var coded exitCoder
	if errors.As(err, &coded) {
		return coded.ExitCode()
	}
	if errors.Is(err, context.Canceled) {
		return 6
	}
	return 1
}

func isUsageError(err error) bool {
	message := err.Error()
	return strings.HasPrefix(message, "unknown command ") || strings.Contains(message, "requires at least") || strings.Contains(message, "accepts ")
}
