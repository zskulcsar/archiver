package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zskulcsar/archiver/internal/domain"
)

type planFile struct {
	FormatVersion        int        `json:"format_version"`
	ArchiveSetID         string     `json:"archive_set_id"`
	UsableCapacity       int64      `json:"usable_capacity_bytes"`
	MinimumSplitSize     string     `json:"minimum_split_size"`
	ResolvedMinSplitSize int64      `json:"resolved_minimum_split_size_bytes"`
	Discs                []planDisc `json:"discs"`
}

type planDisc struct {
	Number int        `json:"number"`
	Used   int64      `json:"used_bytes"`
	Unused int64      `json:"unused_bytes"`
	Parts  []planPart `json:"parts"`
}

type planPart struct {
	LogicalPath   string `json:"logical_path"`
	SymlinkTarget string `json:"symlink_target,omitempty"`
	Offset        int64  `json:"offset_bytes"`
	Size          int64  `json:"size_bytes"`
}

// WritePlanJSON writes a new inspectable archive allocation plan in the output parent.
func WritePlanJSON(config ValidatedConfig, plan domain.ArchivePlan) (string, error) {
	path := filepath.Join(config.Output, config.SetID+".plan.json")
	discs := make([]planDisc, len(plan.Discs))
	for i, disc := range plan.Discs {
		parts := make([]planPart, len(disc.Parts))
		for j, part := range disc.Parts {
			parts[j] = planPart{
				LogicalPath: part.LogicalPath, SymlinkTarget: part.SymlinkTarget,
				Offset: part.Offset, Size: part.Size,
			}
		}
		discs[i] = planDisc{Number: disc.Number, Used: disc.Used, Unused: disc.Unused, Parts: parts}
	}
	content, err := json.MarshalIndent(planFile{
		FormatVersion: 1, ArchiveSetID: config.SetID, UsableCapacity: plan.UsableCapacity,
		MinimumSplitSize: config.RequestedMinSplitSize, ResolvedMinSplitSize: config.MinSplitSize, Discs: discs,
	}, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal archive plan: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return "", fmt.Errorf("stopped before writing archive plan: plan file %q already exists; remove or rename the existing file, or choose a different --archive-name", path)
		}
		return "", fmt.Errorf("create archive plan %q: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	if _, err := file.Write(append(content, '\n')); err != nil {
		return "", fmt.Errorf("write archive plan: %w", err)
	}
	return path, nil
}
