package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type localFilesystemFunc func(string) (bool, error)

func (f localFilesystemFunc) IsLocal(path string) (bool, error) {
	return f(path)
}

// * [x] **P1_CORE_010** Recursive source file collection
// - Description: Collects regular files beneath positional files and directories using paths relative to their common directory.
// - Expected: Every source file has a clean logical path and no directories are returned.
func TestCollectSourceFiles_RecursivelyCollectsRegularFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeSourceFile(t, filepath.Join(root, "photos", "2026", "beach.jpg"))
	writeSourceFile(t, filepath.Join(root, "notes.txt"))

	files, err := CollectSourceFiles([]string{filepath.Join(root, "photos"), filepath.Join(root, "notes.txt")}, localFilesystemFunc(func(string) (bool, error) {
		return true, nil
	}))
	if err != nil {
		t.Fatalf("CollectSourceFiles() error = %v", err)
	}

	got := map[string]string{}
	for _, file := range files {
		got[file.LogicalPath] = file.Path
	}
	want := map[string]string{
		"notes.txt": filepath.Join(root, "notes.txt"),
		filepath.Join("photos", "2026", "beach.jpg"): filepath.Join(root, "photos", "2026", "beach.jpg"),
	}
	if len(got) != len(want) {
		t.Fatalf("CollectSourceFiles() returned %d files, want %d: %#v", len(got), len(want), got)
	}
	for logicalPath, wantPath := range want {
		if gotPath := got[logicalPath]; gotPath != wantPath {
			t.Fatalf("file %q path = %q, want %q", logicalPath, gotPath, wantPath)
		}
	}
}

// * [x] **P1_CORE_011** Invalid source rejection
// - Description: Rejects a missing source and a duplicate logical path before planning.
// - Expected: Collection returns an error for each invalid positional source set.
func TestCollectSourceFiles_RejectsInvalidSources(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	file := filepath.Join(root, "source.txt")
	writeSourceFile(t, file)

	tests := map[string][]string{
		"missing":   {filepath.Join(root, "missing.txt")},
		"duplicate": {file, file},
	}
	for name, paths := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := CollectSourceFiles(paths, localFilesystemFunc(func(string) (bool, error) {
				return true, nil
			}))
			if err == nil {
				t.Fatal("CollectSourceFiles() error = nil, want error")
			}
		})
	}
}

// * [x] **P1_CORE_012** Local filesystem and symlink source enforcement
// - Description: Rejects files on a non-local filesystem and symlinks that could escape the selected source tree.
// - Expected: Collection fails before either source can be included.
func TestCollectSourceFiles_RejectsNonLocalFilesystemAndSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	file := filepath.Join(root, "source.txt")
	writeSourceFile(t, file)

	_, err := CollectSourceFiles([]string{file}, localFilesystemFunc(func(string) (bool, error) {
		return false, nil
	}))
	if err == nil {
		t.Fatal("CollectSourceFiles() error = nil, want error for non-local filesystem")
	}

	target := filepath.Join(t.TempDir(), "outside.txt")
	writeSourceFile(t, target)
	symlink := filepath.Join(root, "external-link.txt")
	if err := os.Symlink(target, symlink); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	_, err = CollectSourceFiles([]string{symlink}, localFilesystemFunc(func(string) (bool, error) {
		return true, nil
	}))
	if err == nil {
		t.Fatal("CollectSourceFiles() error = nil, want error for external symlink")
	}

	_, err = CollectSourceFiles([]string{file}, localFilesystemFunc(func(string) (bool, error) {
		return false, errors.New("filesystem unavailable")
	}))
	if err == nil {
		t.Fatal("CollectSourceFiles() error = nil, want classifier error")
	}
}

// * [x] **P1_CORE_019** Internal symlink preservation
// - Description: Collects a symlink whose resolved regular-file target is also collected as an archive member.
// - Expected: The link has an archive-relative target and does not expose the resolved source path.
func TestCollectSourceFiles_PreservesInternalSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "photo.jpg")
	writeSourceFile(t, target)
	if err := os.Symlink("photo.jpg", filepath.Join(root, "latest.jpg")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	files, err := CollectSourceFiles([]string{root}, localFilesystemFunc(func(string) (bool, error) {
		return true, nil
	}))
	if err != nil {
		t.Fatalf("CollectSourceFiles() error = %v", err)
	}

	if got, want := files, []SourceFile{
		{LogicalPath: "latest.jpg", SymlinkTarget: "photo.jpg"},
		{Path: target, LogicalPath: "photo.jpg"},
	}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("CollectSourceFiles() = %#v, want %#v", got, want)
	}
}

// * [x] **P1_CORE_017** External symlink materialization
// - Description: Materializes local regular-file and directory targets at the symlink logical path, including nested targets.
// - Expected: The collected file paths resolve to targets while their logical paths remain beneath the symlinks that introduced them.
func TestCollectSourceFilesWithPolicy_MaterializesExternalSymlinkTargets(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	external := t.TempDir()
	writeSourceFile(t, filepath.Join(external, "file.txt"))
	writeSourceFile(t, filepath.Join(external, "directory", "child.txt"))
	if err := os.Symlink(filepath.Join(external, "file.txt"), filepath.Join(root, "file-link")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	if err := os.Symlink(filepath.Join(external, "directory"), filepath.Join(root, "directory-link")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	files, err := CollectSourceFilesWithPolicy([]string{root}, localFilesystemFunc(func(string) (bool, error) {
		return true, nil
	}), "materialize")
	if err != nil {
		t.Fatalf("CollectSourceFilesWithPolicy() error = %v", err)
	}

	got := map[string]string{}
	for _, file := range files {
		got[file.LogicalPath] = file.Path
	}
	want := map[string]string{
		"file-link": filepath.Join(external, "file.txt"),
		filepath.Join("directory-link", "child.txt"): filepath.Join(external, "directory", "child.txt"),
	}
	if len(got) != len(want) {
		t.Fatalf("CollectSourceFilesWithPolicy() returned %#v, want %#v", got, want)
	}
	for logicalPath, targetPath := range want {
		if got[logicalPath] != targetPath {
			t.Fatalf("file at %q = %q, want %q", logicalPath, got[logicalPath], targetPath)
		}
	}

	files, err = CollectSourceFilesWithPolicy([]string{filepath.Join(root, "file-link")}, localFilesystemFunc(func(string) (bool, error) {
		return true, nil
	}), "materialize")
	if err != nil {
		t.Fatalf("CollectSourceFilesWithPolicy() source symlink error = %v", err)
	}
	if got, want := files, []SourceFile{{Path: filepath.Join(external, "file.txt"), LogicalPath: "file-link"}}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("CollectSourceFilesWithPolicy() source symlink = %#v, want %#v", got, want)
	}
}

// * [x] **P1_CORE_018** Unsafe materialized symlink rejection
// - Description: Encounters dangling, cyclic, unsupported, and network-backed materialized symlink targets.
// - Expected: Collection rejects every unsafe target before it becomes an archive source.
func TestCollectSourceFilesWithPolicy_RejectsUnsafeMaterializedSymlinks(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		setup      func(t *testing.T, root string)
		filesystem localFilesystemFunc
	}{
		"dangling": {
			setup: func(t *testing.T, root string) {
				t.Helper()
				if err := os.Symlink(filepath.Join(root, "missing"), filepath.Join(root, "link")); err != nil {
					t.Fatalf("Symlink() error = %v", err)
				}
			},
			filesystem: func(string) (bool, error) { return true, nil },
		},
		"cycle": {
			setup: func(t *testing.T, root string) {
				t.Helper()
				if err := os.Symlink(root, filepath.Join(root, "loop")); err != nil {
					t.Fatalf("Symlink() error = %v", err)
				}
			},
			filesystem: func(string) (bool, error) { return true, nil },
		},
		"network": {
			setup: func(t *testing.T, root string) {
				t.Helper()
				target := filepath.Join(t.TempDir(), "target.txt")
				writeSourceFile(t, target)
				if err := os.Symlink(target, filepath.Join(root, "link")); err != nil {
					t.Fatalf("Symlink() error = %v", err)
				}
			},
			filesystem: func(path string) (bool, error) { return filepath.Base(path) != "target.txt", nil },
		},
		"device": {
			setup: func(t *testing.T, root string) {
				t.Helper()
				if err := os.Symlink(os.DevNull, filepath.Join(root, "link")); err != nil {
					t.Fatalf("Symlink() error = %v", err)
				}
			},
			filesystem: func(string) (bool, error) { return true, nil },
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			tc.setup(t, root)
			_, err := CollectSourceFilesWithPolicy([]string{root}, tc.filesystem, "materialize")
			if err == nil {
				t.Fatal("CollectSourceFilesWithPolicy() error = nil, want error")
			}
		})
	}
}

func writeSourceFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("source"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
