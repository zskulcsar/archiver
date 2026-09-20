package adapters

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/zskulcsar/archiver/internal/domain"
)

// * [x] **P1_TAR_001** PAX tar archives an exact planned byte range
// - Description: Builds a two-disc reassembly manifest for one source and writes the first nonzero-offset range to a PAX tar stream.
// - Expected: The tar member contains only the requested bytes under the manifest-assigned part name and ends with the encrypted reassembly manifest.
func TestPAXTarCreator_ArchivesExactPartRangeAndManifest(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "source.bin")
	if err := os.WriteFile(path, []byte("0123456789"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	plan := domain.ArchivePlan{Discs: []domain.Disc{
		{Number: 1, Parts: []domain.Part{{SourcePath: path, LogicalPath: "nested/source.bin", Offset: 2, Size: 4}}},
		{Number: 2, Parts: []domain.Part{{SourcePath: path, LogicalPath: "nested/source.bin", Offset: 6, Size: 4}}},
	}}
	manifest, err := BuildReassemblyManifest(context.Background(), "set-1", plan)
	if err != nil {
		t.Fatalf("BuildReassemblyManifest() error = %v", err)
	}
	var output bytes.Buffer
	if err := (PAXTarCreator{}).CreateArchive(context.Background(), ArchiveRequest{Disc: plan.Discs[0], Reassembly: manifest}, &output); err != nil {
		t.Fatalf("CreateArchive() error = %v", err)
	}

	reader := tar.NewReader(&output)
	header, err := reader.Next()
	if err != nil {
		t.Fatalf("Next() first entry error = %v", err)
	}
	part, ok := manifest.Part(1, "nested/source.bin", 2, 4)
	if !ok {
		t.Fatal("manifest part = missing")
	}
	if got, want := header.Name, part.EntryName; got != want {
		t.Fatalf("tar entry name = %q, want %q", got, want)
	}
	if got, want := header.Format, tar.FormatPAX; got != want {
		t.Fatalf("tar format = %v, want %v", got, want)
	}
	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll(part) error = %v", err)
	}
	if got, want := string(content), "2345"; got != want {
		t.Fatalf("part content = %q, want %q", got, want)
	}
	header, err = reader.Next()
	if err != nil {
		t.Fatalf("Next() manifest error = %v", err)
	}
	if got, want := header.Name, reassemblyManifestPath; got != want {
		t.Fatalf("manifest name = %q, want %q", got, want)
	}
}

// * [x] **P1_TAR_002** PAX tar rejects a source changed after manifest hashing
// - Description: Hashes a complete source into a reassembly manifest, then changes its content before archive creation.
// - Expected: Creation fails rather than packaging bytes that differ from the planned part hash.
func TestPAXTarCreator_RejectsChangedSource(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "source.bin")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	plan := domain.ArchivePlan{Discs: []domain.Disc{{Number: 1, Parts: []domain.Part{{SourcePath: path, LogicalPath: "source.bin", Size: 8}}}}}
	manifest, err := BuildReassemblyManifest(context.Background(), "set-1", plan)
	if err != nil {
		t.Fatalf("BuildReassemblyManifest() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("changed!"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	var output bytes.Buffer
	if err := (PAXTarCreator{}).CreateArchive(context.Background(), ArchiveRequest{Disc: plan.Discs[0], Reassembly: manifest}, &output); err == nil {
		t.Fatal("CreateArchive() error = nil, want changed-source error")
	}
}

// * [x] **P1_TAR_003** PAX tar preserves symlinks with archive-relative targets
// - Description: Writes a nested logical symlink that targets a member at the archive root.
// - Expected: The tar header is a symlink and its target is relative to the link's archive directory.
func TestPAXTarCreator_WritesRelativeSymlink(t *testing.T) {
	t.Parallel()

	request := ArchiveRequest{Disc: domain.Disc{Number: 1, Parts: []domain.Part{{LogicalPath: "nested/latest", SymlinkTarget: "photo.jpg"}}}, Reassembly: ReassemblyManifest{FormatVersion: 1, ArchiveSetID: "set-1"}}
	var output bytes.Buffer
	if err := (PAXTarCreator{}).CreateArchive(context.Background(), request, &output); err != nil {
		t.Fatalf("CreateArchive() error = %v", err)
	}
	reader := tar.NewReader(&output)
	header, err := reader.Next()
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if got, want := header.Typeflag, byte(tar.TypeSymlink); got != want {
		t.Fatalf("symlink type = %q, want %q", got, want)
	}
	if got, want := header.Linkname, "../photo.jpg"; got != want {
		t.Fatalf("symlink target = %q, want %q", got, want)
	}
}
