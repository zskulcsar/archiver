package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zskulcsar/archiver/internal/adapters"
	"github.com/zskulcsar/archiver/internal/domain"
)

const persistedFormatVersion = 1

var phaseOneToolBundle = adapters.ToolIdentity{Name: "not-included", Version: "phase-1.2 placeholder"}

type artifactMetadata struct {
	Name   string                `json:"name"`
	Kind   string                `json:"kind"`
	Size   int64                 `json:"size_bytes"`
	SHA256 string                `json:"sha256"`
	Format string                `json:"format"`
	Tool   adapters.ToolIdentity `json:"tool"`
}

type discManifest struct {
	FormatVersion        int                   `json:"format_version"`
	ArchiveSetID         string                `json:"archive_set_id"`
	DiscNumber           int                   `json:"disc_number"`
	PlannedCapacity      int64                 `json:"planned_capacity_bytes"`
	PlannedPayload       int64                 `json:"planned_payload_bytes"`
	PlannedUnused        int64                 `json:"planned_unused_bytes"`
	Artifacts            []artifactMetadata    `json:"artifacts"`
	RecoveryInstructions string                `json:"recovery_instructions"`
	RecoveryToolBundle   adapters.ToolIdentity `json:"recovery_tool_bundle"`
}

type archiveSetManifest struct {
	FormatVersion      int                   `json:"format_version"`
	ArchiveSetID       string                `json:"archive_set_id"`
	PlannedCapacity    int64                 `json:"planned_capacity_bytes"`
	DiscManifests      []string              `json:"disc_manifests"`
	RecoveryToolBundle adapters.ToolIdentity `json:"recovery_tool_bundle"`
}

type finalReport struct {
	FormatVersion      int                     `json:"format_version"`
	ArchiveSetID       string                  `json:"archive_set_id"`
	PlannedCapacity    int64                   `json:"planned_capacity_bytes"`
	ActualCapacity     int64                   `json:"actual_capacity_bytes"`
	Images             []artifactMetadata      `json:"images"`
	Tools              []adapters.ToolIdentity `json:"tools"`
	RecoveryToolBundle adapters.ToolIdentity   `json:"recovery_tool_bundle"`
	Verification       string                  `json:"verification"`
}

type discResult struct {
	discManifestName string
	image            artifactMetadata
	tools            []adapters.ToolIdentity
}

func metadataForArtifact(artifact adapters.Artifact, kind string) (artifactMetadata, error) {
	content, err := os.ReadFile(artifact.Path)
	if err != nil {
		return artifactMetadata{}, fmt.Errorf("read %s artifact: %w", kind, err)
	}
	hash := sha256.Sum256(content)
	format := artifact.Format
	if format == "" {
		format = kind
	}
	tool := artifact.Tool
	if tool.Name == "" {
		tool = adapters.ToolIdentity{Name: "unspecified", Version: "unspecified"}
	}
	return artifactMetadata{
		Name: filepath.Base(artifact.Path), Kind: kind, Size: int64(len(content)),
		SHA256: hex.EncodeToString(hash[:]), Format: format, Tool: tool,
	}, nil
}

func writeDiscMetadata(stagingPath string, setID string, plannedCapacity int64, disc domain.Disc, artifacts []artifactMetadata) (string, []adapters.Artifact, error) {
	instructionsName := fmt.Sprintf("disc-%02d-recovery.txt", disc.Number)
	instructionsPath := filepath.Join(stagingPath, instructionsName)
	if err := writeRecoveryInstructions(instructionsPath, setID, disc.Number, artifacts); err != nil {
		return "", nil, err
	}
	manifestName := fmt.Sprintf("disc-%02d-manifest.json", disc.Number)
	manifest := discManifest{
		FormatVersion: persistedFormatVersion, ArchiveSetID: setID, DiscNumber: disc.Number,
		PlannedCapacity: plannedCapacity, PlannedPayload: disc.Used, PlannedUnused: disc.Unused,
		Artifacts: artifacts, RecoveryInstructions: instructionsName, RecoveryToolBundle: phaseOneToolBundle,
	}
	manifestPath := filepath.Join(stagingPath, manifestName)
	if err := writeJSON(manifestPath, manifest); err != nil {
		return "", nil, err
	}
	return manifestName, []adapters.Artifact{
		{Path: manifestPath, Format: "application/json"},
		{Path: instructionsPath, Format: "text/plain"},
	}, nil
}

func writeRecoveryInstructions(path, setID string, discNumber int, artifacts []artifactMetadata) error {
	var instructions strings.Builder
	fmt.Fprintf(&instructions, "Archive set: %s\nDisc: %d\n\n", setID, discNumber)
	instructions.WriteString("Recovery steps:\n")
	instructions.WriteString("1. Verify each listed artifact against its SHA-256 value.\n")
	instructions.WriteString("2. Use a compatible tool for the recorded artifact format to repair and decrypt this disc.\n")
	instructions.WriteString("3. Extract the recovered payload to the chosen destination.\n\nArtifacts:\n")
	for _, artifact := range artifacts {
		fmt.Fprintf(&instructions, "- %s: format %s; tool %s (%s); SHA-256 %s\n", artifact.Name, artifact.Format, artifact.Tool.Name, artifact.Tool.Version, artifact.SHA256)
	}
	fmt.Fprintf(&instructions, "\nRecovery tool bundle: %s (%s)\n", phaseOneToolBundle.Name, phaseOneToolBundle.Version)
	return os.WriteFile(path, []byte(instructions.String()), 0o600)
}

func writeArchiveSetMetadata(stagingPath, setID string, plan domain.ArchivePlan, results []discResult) error {
	manifestNames := make([]string, len(results))
	for i, result := range results {
		manifestNames[i] = result.discManifestName
	}
	return writeJSON(filepath.Join(stagingPath, "archive-set-manifest.json"), archiveSetManifest{
		FormatVersion: persistedFormatVersion, ArchiveSetID: setID,
		PlannedCapacity: plan.UsableCapacity * int64(len(plan.Discs)),
		DiscManifests:   manifestNames, RecoveryToolBundle: phaseOneToolBundle,
	})
}

func writeFinalReport(stagingPath, setID string, plan domain.ArchivePlan, results []discResult) error {
	images := make([]artifactMetadata, len(results))
	tools := make([]adapters.ToolIdentity, 0, len(results))
	var actualCapacity int64
	for i, result := range results {
		images[i] = result.image
		actualCapacity += result.image.Size
		for _, tool := range result.tools {
			tools = appendTool(tools, tool)
		}
	}
	tools = appendTool(tools, phaseOneToolBundle)
	return writeJSON(filepath.Join(stagingPath, "report.json"), finalReport{
		FormatVersion: persistedFormatVersion, ArchiveSetID: setID,
		PlannedCapacity: plan.UsableCapacity * int64(len(plan.Discs)), ActualCapacity: actualCapacity,
		Images: images, Tools: tools, RecoveryToolBundle: phaseOneToolBundle, Verification: "passed",
	})
}

func appendTool(tools []adapters.ToolIdentity, tool adapters.ToolIdentity) []adapters.ToolIdentity {
	for _, existing := range tools {
		if existing == tool {
			return tools
		}
	}
	return append(tools, tool)
}

func writeJSON(path string, value any) error {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	if err := os.WriteFile(path, append(content, '\n'), 0o600); err != nil {
		return fmt.Errorf("write metadata %q: %w", filepath.Base(path), err)
	}
	return nil
}
