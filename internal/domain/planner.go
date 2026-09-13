package domain

import (
	"fmt"
	"sort"
)

// Symlink is an archive member whose target is another archive-relative path.
type Symlink struct {
	LogicalPath string
	TargetPath  string
}

// Source is a validated source file or symlink represented in an archive plan.
type Source struct {
	SourcePath    string
	LogicalPath   string
	SymlinkTarget string
	Size          int64
}

// Part is a source-file portion or symlink member assigned to one disc.
type Part struct {
	SourcePath    string
	LogicalPath   string
	SymlinkTarget string
	Offset        int64
	Size          int64
}

// Disc is one independently recoverable archive payload allocation.
type Disc struct {
	Number int
	Parts  []Part
	Used   int64
	Unused int64
}

// ArchivePlan is the deterministic assignment of archive members to discs.
type ArchivePlan struct {
	UsableCapacity int64
	Discs          []Disc
}

// Plan assigns sources to discs in canonical logical-path order.
func Plan(sources []Source, usableCapacity int64) (ArchivePlan, error) {
	return PlanWithMinimumSplitSize(sources, usableCapacity, 0)
}

// PlanWithMinimumSplitSize assigns sources using the supplied resolved split threshold.
func PlanWithMinimumSplitSize(sources []Source, usableCapacity, minimumSplitSize int64) (ArchivePlan, error) {
	if usableCapacity <= 0 {
		return ArchivePlan{}, fmt.Errorf("usable capacity must be positive")
	}
	if minimumSplitSize < 0 || minimumSplitSize > usableCapacity {
		return ArchivePlan{}, fmt.Errorf("minimum split size must be between zero and usable capacity")
	}

	ordered := append([]Source(nil), sources...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].LogicalPath < ordered[j].LogicalPath
	})
	for _, source := range ordered {
		if source.LogicalPath == "" {
			return ArchivePlan{}, fmt.Errorf("source logical path must not be empty")
		}
		if source.Size < 0 {
			return ArchivePlan{}, fmt.Errorf("source %q has a negative size", source.LogicalPath)
		}
		if source.SymlinkTarget != "" && source.Size != 0 {
			return ArchivePlan{}, fmt.Errorf("symlink source %q must have zero size", source.LogicalPath)
		}
	}
	if len(ordered) == 0 {
		return ArchivePlan{UsableCapacity: usableCapacity}, nil
	}

	plan := ArchivePlan{UsableCapacity: usableCapacity}
	current := Disc{Number: 1}
	for _, source := range ordered {
		remaining := source.Size
		offset := int64(0)
		for {
			available := usableCapacity - current.Used
			if remaining <= available {
				current.Parts = append(current.Parts, Part{
					SourcePath:    source.SourcePath,
					LogicalPath:   source.LogicalPath,
					SymlinkTarget: source.SymlinkTarget,
					Offset:        offset,
					Size:          remaining,
				})
				current.Used += remaining
				break
			}

			if available < minimumSplitSize {
				if current.Used == 0 {
					return ArchivePlan{}, fmt.Errorf("source %q cannot fit within usable capacity without a part smaller than 1GB", source.LogicalPath)
				}
				current.Unused = available
				plan.Discs = append(plan.Discs, current)
				current = Disc{Number: len(plan.Discs) + 1}
				continue
			}

			current.Parts = append(current.Parts, Part{
				SourcePath:    source.SourcePath,
				LogicalPath:   source.LogicalPath,
				SymlinkTarget: source.SymlinkTarget,
				Offset:        offset,
				Size:          available,
			})
			current.Used += available
			plan.Discs = append(plan.Discs, current)
			offset += available
			remaining -= available
			current = Disc{Number: len(plan.Discs) + 1}
		}
	}
	current.Unused = usableCapacity - current.Used
	plan.Discs = append(plan.Discs, current)

	return plan, nil
}
