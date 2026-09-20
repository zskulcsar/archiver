// Package cli adapts command-line input to Archiver application behavior.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zskulcsar/archiver/internal/adapters"
	"github.com/zskulcsar/archiver/internal/app"
	"github.com/zskulcsar/archiver/internal/domain"
	"github.com/zskulcsar/archiver/internal/observability"
	platformlinux "github.com/zskulcsar/archiver/internal/platform/linux"
	"go.opentelemetry.io/otel/trace"
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
	telemetry := &telemetryRuntime{stderr: stderr, buildInfo: buildInfo}
	root := newRootCommand(buildInfo, services, telemetry)
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)

	err := root.Execute()
	telemetry.shutdown()
	if err != nil {
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
	return newRootCommand(buildInfo, defaultServices(), &telemetryRuntime{buildInfo: buildInfo})
}

func newRootCommand(buildInfo BuildInfo, services Services, telemetry *telemetryRuntime) *cobra.Command {
	var otelEndpoint string
	root := &cobra.Command{
		Use:           "archiver",
		Short:         "Create encrypted, multi-disc archive images",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			ctx := telemetry.initialize(cmd.Context(), otelEndpoint)
			ctx, span := observability.Start(ctx, "archiver."+cmd.Name())
			telemetry.span = span
			cmd.SetContext(ctx)
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	root.PersistentFlags().StringVar(&otelEndpoint, "otel-endpoint", "", "OTLP/HTTP endpoint for optional telemetry, such as http://127.0.0.1:4318")

	root.AddCommand(newVersionCommand(buildInfo))
	root.AddCommand(newBackendsCommand(services))
	root.AddCommand(newPlanCommand(services))
	root.AddCommand(newCreateCommand(services))
	root.AddCommand(newVerifyCommand(services))
	return root
}

type telemetryRuntime struct {
	stderr     io.Writer
	buildInfo  BuildInfo
	shutdownFn func(context.Context) error
	span       trace.Span
}

func (r *telemetryRuntime) initialize(ctx context.Context, endpoint string) context.Context {
	if r.shutdownFn != nil {
		return ctx
	}
	resolved := observability.ResolveEndpoint(endpoint, os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	shutdown, err := observability.Initialize(ctx, observability.Config{Endpoint: resolved, Version: r.buildInfo.Version, Revision: r.buildInfo.Revision})
	if err != nil {
		if r.stderr != nil {
			_, _ = fmt.Fprintf(r.stderr, "warning: telemetry disabled: %v\n", err)
		}
		return ctx
	}
	r.shutdownFn = shutdown
	return ctx
}

func (r *telemetryRuntime) shutdown() {
	if r.span != nil {
		r.span.End()
	}
	if r.shutdownFn == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), observability.ShutdownTimeout)
	defer cancel()
	if err := r.shutdownFn(ctx); err != nil && r.stderr != nil {
		_, _ = fmt.Fprintf(r.stderr, "warning: telemetry export failed: %v\n", err)
	}
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
	Path         string
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
	statuses := platformlinux.DiscoverToolStatuses(context.Background(), platformlinux.ToolPaths{}, exec.LookPath, platformlinux.ExecRunner{})
	gpg := statuses["gpg"]
	par2 := statuses["par2"]
	xorriso := statuses["xorriso"]
	return []Backend{
		{Name: gpg.Tool.Name, Path: gpg.Tool.Path, Version: gpg.Tool.Version, Available: gpg.Available, Capabilities: []string{"symmetric-encryption", "decrypt"}},
		{Name: par2.Tool.Name, Path: par2.Tool.Path, Version: par2.Tool.Version, Available: par2.Available, Capabilities: []string{"parity-create", "parity-verify", "parity-repair"}},
		{Name: xorriso.Tool.Name, Path: xorriso.Tool.Path, Version: xorriso.Tool.Version, Available: xorriso.Available, Capabilities: []string{"image-create", "image-verify", "writer-discovery", "burn-disabled"}},
	}, nil
}

func (s portableServices) Plan(ctx context.Context, config app.ValidatedConfig) (domain.ArchivePlan, error) {
	ctx, span := observability.Start(ctx, "archiver.plan")
	defer span.End()
	sources, err := s.sources(ctx, config)
	if err != nil {
		return domain.ArchivePlan{}, err
	}
	usableCapacity, err := (app.LinuxISOProfile{}).UsablePayloadCapacity(config.CapacityBytes, config.LossPercent, len(sources))
	if err != nil {
		return domain.ArchivePlan{}, fmt.Errorf("calculate Linux image capacity: %w", err)
	}
	minimumSplitSize, err := domain.ParseMinimumSplitSize(config.RequestedMinSplitSize, usableCapacity)
	if err != nil {
		return domain.ArchivePlan{}, fmt.Errorf("resolve minimum split size: %w", err)
	}
	plan, err := domain.PlanWithMinimumSplitSize(sources, usableCapacity, minimumSplitSize)
	if err != nil {
		return domain.ArchivePlan{}, fmt.Errorf("plan archive: %w", err)
	}
	return plan, nil
}

func (s portableServices) Create(ctx context.Context, config app.ValidatedConfig, events app.EventSink) error {
	plan, err := s.Plan(ctx, config)
	if err != nil {
		return err
	}
	discoveryCtx, discoverySpan := observability.Start(ctx, "archiver.backend.discover")
	tools, err := platformlinux.DiscoverTools(discoveryCtx, platformlinux.ToolPaths{}, exec.LookPath, platformlinux.ExecRunner{})
	observability.RecordError(discoverySpan, err)
	discoverySpan.End()
	if err != nil {
		return dependencyError{err}
	}
	if config.PassphraseFile == "" {
		return configurationError{errors.New("--passphrase-file is required for non-interactive Linux creation")}
	}
	if err := platformlinux.ValidatePassphraseFile(config.PassphraseFile); err != nil {
		return configurationError{err}
	}
	return app.Create(ctx, app.CreateRequest{
		Config: config, Plan: plan, Backends: platformlinux.NewBackends(tools, config.PassphraseFile, config.LossPercent), Events: events,
	})
}

func (portableServices) Verify(ctx context.Context, archiveSetPath string, events app.EventSink) error {
	discoveryCtx, discoverySpan := observability.Start(ctx, "archiver.backend.discover")
	tools, err := platformlinux.DiscoverTools(discoveryCtx, platformlinux.ToolPaths{}, exec.LookPath, platformlinux.ExecRunner{})
	observability.RecordError(discoverySpan, err)
	discoverySpan.End()
	if err != nil {
		return dependencyError{err}
	}
	entries, err := os.ReadDir(archiveSetPath)
	if err != nil {
		return fmt.Errorf("read archive set: %w", err)
	}
	backends := platformlinux.NewBackends(tools, "", 0)
	images := 0
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".img" {
			continue
		}
		images++
		if err := backends.VerifyImage(ctx, adapters.Artifact{Path: filepath.Join(archiveSetPath, entry.Name())}); err != nil {
			return app.VerificationError{Err: err}
		}
		if events != nil {
			if err := events.Emit(app.Event{Type: "image_verified", Disc: images}); err != nil {
				return err
			}
		}
	}
	if images == 0 {
		return fmt.Errorf("archive set contains no image files")
	}
	return nil
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
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\tavailable=%t\tversion=%s\tpath=%s\tcapabilities=%s\n", backend.Name, backend.Available, backend.Version, backend.Path, strings.Join(backend.Capabilities, ",")); err != nil {
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
			plan, err := services.Plan(cmd.Context(), validated)
			if err != nil {
				return err
			}
			if err := writeCreateSummary(cmd.OutOrStdout(), validated, plan); err != nil {
				return err
			}
			events, closeEvents, err := openConfigEvents(validated)
			if err != nil {
				return configurationError{err}
			}
			defer closeEvents()
			validated.EventsFile = ""
			terminalEvents := terminalEventSink{events: events, output: cmd.OutOrStdout(), discCount: len(plan.Discs)}
			if err := services.Create(cmd.Context(), validated, terminalEvents); err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "Archive set created")
			return err
		},
	}
	addArchiveConfigFlags(command, &config, true)
	return command
}

type terminalEventSink struct {
	events    app.EventSink
	output    io.Writer
	discCount int
}

func (s terminalEventSink) Emit(event app.Event) error {
	if s.events != nil {
		if err := s.events.Emit(event); err != nil {
			return err
		}
	}
	message := formatTerminalEvent(event, s.discCount)
	if message == "" {
		return nil
	}
	_, err := fmt.Fprintln(s.output, message)
	return err
}

func writeCreateSummary(output io.Writer, config app.ValidatedConfig, plan domain.ArchivePlan) error {
	files, bytes := sourcePlanStatistics(plan)
	_, err := fmt.Fprintf(output, "Archive parameters:\n  Capacity: %s\n  Usable payload per disc: %d bytes\n  Loss tolerance: %d%%\n  Minimum split size: %s\nSource: %d files, %d bytes\nPlan: %d discs\n", config.Capacity, plan.UsableCapacity, config.LossPercent, config.RequestedMinSplitSize, files, bytes, len(plan.Discs))
	return err
}

func sourcePlanStatistics(plan domain.ArchivePlan) (int, int64) {
	files := make(map[string]int64)
	for _, disc := range plan.Discs {
		for _, part := range disc.Parts {
			if part.SymlinkTarget != "" {
				continue
			}
			if end := part.Offset + part.Size; end > files[part.LogicalPath] {
				files[part.LogicalPath] = end
			}
		}
	}
	var bytes int64
	for _, size := range files {
		bytes += size
	}
	return len(files), bytes
}

func formatTerminalEvent(event app.Event, discCount int) string {
	disc := fmt.Sprintf("Disc %d/%d", event.Disc, discCount)
	switch event.Type {
	case "validation_started":
		return "Validating and hashing source ranges..."
	case "validation_completed":
		return "Source ranges validated."
	case "staging_started":
		return "Preparing archive staging area..."
	case "archive_started":
		return disc + ": creating archive and encrypting"
	case "archive_completed":
		return disc + ": archive created and encrypted"
	case "parity_started":
		return disc + ": creating PAR2 recovery data"
	case "parity_completed":
		return disc + ": PAR2 recovery data verified"
	case "image_started":
		return disc + ": creating image"
	case "image_verified":
		return disc + ": image verified"
	case "disc_completed":
		return disc + ": completed"
	case "published":
		return "Archive set published."
	default:
		return ""
	}
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
