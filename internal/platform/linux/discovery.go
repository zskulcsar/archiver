package linux

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// ToolPaths contains optional explicit paths for Linux image-only tools.
type ToolPaths struct {
	GPG     string
	PAR2    string
	Xorriso string
}

// Tool identifies a discovered executable and its reported version.
type Tool struct {
	Name    string
	Path    string
	Version string
}

// Tools contains the Linux tools required for image-only archive creation.
type Tools struct {
	GPG     Tool
	PAR2    Tool
	Xorriso Tool
}

// ToolStatus records whether a tool was discovered and queried successfully.
type ToolStatus struct {
	Tool      Tool
	Available bool
	Error     string
}

// CommandRunner runs one command without invoking a shell.
type CommandRunner interface {
	Run(ctx context.Context, path string, args ...string) ([]byte, error)
}

type toolSpec struct {
	name        string
	explicit    string
	lookupNames []string
	versionArg  []string
}

var versionPattern = regexp.MustCompile(`(?i)(?:version|gnupg|7-zip|xorriso)[^0-9]*([0-9]+(?:\.[0-9]+)+)`) //nolint:gochecknoglobals // Compiled once for discovery.

// DiscoverTools resolves and queries every tool required for image-only creation.
func DiscoverTools(ctx context.Context, paths ToolPaths, lookup func(string) (string, error), runner CommandRunner) (Tools, error) {
	if lookup == nil {
		return Tools{}, fmt.Errorf("tool lookup is required")
	}
	if runner == nil {
		return Tools{}, fmt.Errorf("command runner is required")
	}

	statuses := DiscoverToolStatuses(ctx, paths, lookup, runner)
	tools := make([]Tool, 0, len(statuses))
	missing := make([]string, 0)
	for _, name := range []string{"gpg", "par2", "xorriso"} {
		status := statuses[name]
		if !status.Available {
			missing = append(missing, name)
			continue
		}
		tools = append(tools, status.Tool)
	}
	if len(missing) > 0 {
		return Tools{}, fmt.Errorf("missing or unusable required Linux tools: %s; install gnupg, par2cmdline, and xorriso", strings.Join(missing, ", "))
	}

	return Tools{GPG: tools[0], PAR2: tools[1], Xorriso: tools[2]}, nil
}

// DiscoverToolStatuses reports independent discovery status for every image-only tool.
func DiscoverToolStatuses(ctx context.Context, paths ToolPaths, lookup func(string) (string, error), runner CommandRunner) map[string]ToolStatus {
	statuses := make(map[string]ToolStatus, 3)
	if lookup == nil || runner == nil {
		for _, name := range []string{"gpg", "par2", "xorriso"} {
			statuses[name] = ToolStatus{Tool: Tool{Name: name}, Error: "tool discovery dependencies are unavailable"}
		}
		return statuses
	}
	for _, spec := range []toolSpec{
		{name: "gpg", explicit: paths.GPG, lookupNames: []string{"gpg"}, versionArg: []string{"--version"}},
		{name: "par2", explicit: paths.PAR2, lookupNames: []string{"par2"}, versionArg: []string{"--version"}},
		{name: "xorriso", explicit: paths.Xorriso, lookupNames: []string{"xorriso"}, versionArg: []string{"-version"}},
	} {
		path, err := resolveTool(spec, lookup)
		if err != nil {
			statuses[spec.name] = ToolStatus{Tool: Tool{Name: spec.name}, Error: err.Error()}
			continue
		}
		output, err := runner.Run(ctx, path, spec.versionArg...)
		if err != nil {
			statuses[spec.name] = ToolStatus{Tool: Tool{Name: spec.name, Path: path}, Error: err.Error()}
			continue
		}
		statuses[spec.name] = ToolStatus{Tool: Tool{Name: spec.name, Path: path, Version: parseVersion(string(output))}, Available: true}
	}
	return statuses
}

func resolveTool(spec toolSpec, lookup func(string) (string, error)) (string, error) {
	if spec.explicit != "" {
		return filepath.Clean(spec.explicit), nil
	}
	var lastError error
	for _, name := range spec.lookupNames {
		path, err := lookup(name)
		if err == nil {
			return path, nil
		}
		lastError = err
	}
	return "", lastError
}

func parseVersion(output string) string {
	match := versionPattern.FindStringSubmatch(output)
	if len(match) == 2 {
		return match[1]
	}
	return "unknown"
}
