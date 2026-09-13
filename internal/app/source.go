package app

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LocalFilesystem identifies whether a path is on a filesystem that can be read locally.
type LocalFilesystem interface {
	IsLocal(path string) (bool, error)
}

// SourceFile identifies a validated regular file or symlink archive member.
type SourceFile struct {
	Path          string
	LogicalPath   string
	SymlinkTarget string
}

// CollectSourceFiles validates positional source paths and collects archive members.
func CollectSourceFiles(sourcePaths []string, filesystems LocalFilesystem) ([]SourceFile, error) {
	return CollectSourceFilesWithPolicy(sourcePaths, filesystems, "")
}

// CollectSourceFilesWithPolicy validates positional source paths and collects archive members.
func CollectSourceFilesWithPolicy(sourcePaths []string, filesystems LocalFilesystem, externalSymlink string) ([]SourceFile, error) {
	if len(sourcePaths) == 0 {
		return nil, fmt.Errorf("at least one source path is required")
	}
	if filesystems == nil {
		return nil, fmt.Errorf("local filesystem classifier is required")
	}
	if externalSymlink != "" && externalSymlink != "materialize" {
		return nil, fmt.Errorf("invalid external symlink policy %q", externalSymlink)
	}

	paths := make([]string, len(sourcePaths))
	bases := make([]string, len(sourcePaths))
	for index, sourcePath := range sourcePaths {
		path, err := filepath.Abs(sourcePath)
		if err != nil {
			return nil, fmt.Errorf("resolve source path %q: %w", sourcePath, err)
		}
		info, err := os.Lstat(path)
		if err != nil {
			return nil, fmt.Errorf("inspect source path %q: %w", sourcePath, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			bases[index] = filepath.Dir(path)
			if externalSymlink == "materialize" {
				resolved, err := filepath.EvalSymlinks(path)
				if err != nil {
					return nil, fmt.Errorf("resolve symlink %q: %w", sourcePath, err)
				}
				info, err = os.Stat(resolved)
				if err != nil {
					return nil, fmt.Errorf("inspect symlink target %q: %w", sourcePath, err)
				}
			}
		}
		if info.Mode()&os.ModeSymlink == 0 && !info.IsDir() && !info.Mode().IsRegular() {
			return nil, fmt.Errorf("source path %q is not a regular file or directory", sourcePath)
		}
		paths[index] = path
		if bases[index] == "" {
			bases[index] = path
		}
		if !info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			bases[index] = filepath.Dir(path)
		}
	}

	commonDirectory, err := commonDirectory(bases)
	if err != nil {
		return nil, err
	}
	files := make([]SourceFile, 0)
	links := make([]symlinkCandidate, 0)
	seen := make(map[string]struct{})
	for _, sourcePath := range paths {
		logicalPath, err := filepath.Rel(commonDirectory, sourcePath)
		if err != nil {
			return nil, fmt.Errorf("derive logical path for %q: %w", sourcePath, err)
		}
		err = collectSourcePath(sourcePath, logicalPath, filesystems, externalSymlink, nil, seen, &files, &links)
		if err != nil {
			return nil, err
		}
	}
	regularPaths := make(map[string]string, len(files))
	for _, file := range files {
		resolved, err := filepath.EvalSymlinks(file.Path)
		if err != nil {
			return nil, fmt.Errorf("resolve source file %q: %w", file.Path, err)
		}
		regularPaths[resolved] = file.LogicalPath
	}
	for _, link := range links {
		resolved, err := filepath.EvalSymlinks(link.path)
		if err != nil {
			return nil, fmt.Errorf("resolve symlink %q: %w", link.path, err)
		}
		target, exists := regularPaths[resolved]
		if !exists {
			return nil, fmt.Errorf("symlink source path %q targets a member outside the archive", link.path)
		}
		if _, exists := seen[link.logicalPath]; exists {
			return nil, fmt.Errorf("duplicate logical source path %q", link.logicalPath)
		}
		seen[link.logicalPath] = struct{}{}
		files = append(files, SourceFile{LogicalPath: link.logicalPath, SymlinkTarget: target})
	}
	sort.Slice(files, func(left, right int) bool {
		return files[left].LogicalPath < files[right].LogicalPath
	})
	return files, nil
}

type symlinkCandidate struct {
	path        string
	logicalPath string
}

func collectSourcePath(path, logicalPath string, filesystems LocalFilesystem, externalSymlink string, ancestors []fs.FileInfo, seen map[string]struct{}, files *[]SourceFile, links *[]symlinkCandidate) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect source path %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if externalSymlink != "materialize" {
			if !safeLogicalPath(logicalPath) {
				return fmt.Errorf("source path %q produces unsafe logical path %q", path, logicalPath)
			}
			*links = append(*links, symlinkCandidate{path: path, logicalPath: logicalPath})
			return nil
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return fmt.Errorf("resolve symlink %q: %w", path, err)
		}
		return collectSourcePath(resolved, logicalPath, filesystems, externalSymlink, ancestors, seen, files, links)
	}
	isLocal, err := filesystems.IsLocal(path)
	if err != nil {
		return fmt.Errorf("classify filesystem for %q: %w", path, err)
	}
	if !isLocal {
		return fmt.Errorf("source path %q is not on a local filesystem", path)
	}
	if info.IsDir() {
		for _, ancestor := range ancestors {
			if os.SameFile(ancestor, info) {
				return fmt.Errorf("symlink cycle at source path %q", path)
			}
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return fmt.Errorf("read source directory %q: %w", path, err)
		}
		ancestors = append(ancestors, info)
		for _, entry := range entries {
			if err := collectSourcePath(filepath.Join(path, entry.Name()), filepath.Join(logicalPath, entry.Name()), filesystems, externalSymlink, ancestors, seen, files, links); err != nil {
				return err
			}
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source path %q is not a regular file", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open source file %q: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close source file %q: %w", path, err)
	}
	if !safeLogicalPath(logicalPath) {
		return fmt.Errorf("source path %q produces unsafe logical path %q", path, logicalPath)
	}
	if _, exists := seen[logicalPath]; exists {
		return fmt.Errorf("duplicate logical source path %q", logicalPath)
	}
	seen[logicalPath] = struct{}{}
	*files = append(*files, SourceFile{Path: path, LogicalPath: logicalPath})
	return nil
}

func commonDirectory(paths []string) (string, error) {
	common := paths[0]
	for _, path := range paths[1:] {
		for {
			relative, err := filepath.Rel(common, path)
			if err != nil {
				return "", fmt.Errorf("derive common source directory: %w", err)
			}
			if relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				break
			}
			parent := filepath.Dir(common)
			if parent == common {
				break
			}
			common = parent
		}
	}
	return common, nil
}

func safeLogicalPath(path string) bool {
	return path != "." && !filepath.IsAbs(path) && path != ".." && !strings.HasPrefix(path, ".."+string(filepath.Separator))
}
