package adapters

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/zskulcsar/archiver/internal/domain"
	"github.com/zskulcsar/archiver/internal/observability"
	"go.opentelemetry.io/otel/attribute"
)

const reassemblyManifestPath = ".archiver/reassembly-manifest.json"

// ReassemblyManifest describes every regular-file range in an encrypted archive set.
type ReassemblyManifest struct {
	FormatVersion int              `json:"format_version"`
	ArchiveSetID  string           `json:"archive_set_id"`
	Parts         []ReassemblyPart `json:"parts"`
}

// ReassemblyPart identifies a verified range and its member name in one disc archive.
type ReassemblyPart struct {
	DiscNumber   int    `json:"disc_number"`
	LogicalPath  string `json:"logical_path"`
	EntryName    string `json:"entry_name"`
	Offset       int64  `json:"offset_bytes"`
	Size         int64  `json:"size_bytes"`
	OriginalSize int64  `json:"original_size_bytes"`
	SHA256       string `json:"sha256"`
}

// Part returns the matching manifest record for a planned source range.
func (m ReassemblyManifest) Part(discNumber int, logicalPath string, offset, size int64) (ReassemblyPart, bool) {
	for _, part := range m.Parts {
		if part.DiscNumber == discNumber && part.LogicalPath == logicalPath && part.Offset == offset && part.Size == size {
			return part, true
		}
	}
	return ReassemblyPart{}, false
}

// BuildReassemblyManifest hashes every planned regular-file range before archive creation.
func BuildReassemblyManifest(ctx context.Context, setID string, plan domain.ArchivePlan) (ReassemblyManifest, error) {
	ctx, span := observability.Start(ctx, "archiver.source.hash", attribute.Int("archiver.disc.count", len(plan.Discs)))
	defer span.End()
	manifest := ReassemblyManifest{FormatVersion: 1, ArchiveSetID: setID}
	for _, disc := range plan.Discs {
		for _, part := range disc.Parts {
			if part.SymlinkTarget != "" {
				continue
			}
			info, err := os.Stat(part.SourcePath)
			if err != nil {
				return ReassemblyManifest{}, fmt.Errorf("inspect source %q: %w", part.LogicalPath, err)
			}
			if err := validatePartRange(part, info.Size()); err != nil {
				return ReassemblyManifest{}, err
			}
			hash, err := hashPart(ctx, part)
			if err != nil {
				return ReassemblyManifest{}, err
			}
			manifest.Parts = append(manifest.Parts, ReassemblyPart{
				DiscNumber: disc.Number, LogicalPath: part.LogicalPath, EntryName: partEntryName(part, info.Size()),
				Offset: part.Offset, Size: part.Size, OriginalSize: info.Size(), SHA256: hash,
			})
		}
	}
	sort.Slice(manifest.Parts, func(i, j int) bool {
		left, right := manifest.Parts[i], manifest.Parts[j]
		if left.DiscNumber != right.DiscNumber {
			return left.DiscNumber < right.DiscNumber
		}
		if left.LogicalPath != right.LogicalPath {
			return left.LogicalPath < right.LogicalPath
		}
		return left.Offset < right.Offset
	})
	return manifest, nil
}

// PAXTarCreator streams planned source ranges into a PAX tar archive.
type PAXTarCreator struct{}

// CreateArchive writes one independently recoverable PAX tar payload and its reassembly manifest.
func (PAXTarCreator) CreateArchive(ctx context.Context, request ArchiveRequest, output io.Writer) error {
	ctx, span := observability.Start(ctx, "archiver.pax.create", attribute.Int("archiver.disc.number", request.Disc.Number), attribute.Int("archiver.part.count", len(request.Disc.Parts)))
	defer span.End()
	writer := tar.NewWriter(output)
	for _, part := range request.Disc.Parts {
		if err := ctx.Err(); err != nil {
			return err
		}
		if part.SymlinkTarget != "" {
			if err := writeSymlink(writer, part); err != nil {
				return err
			}
			continue
		}
		entry, ok := request.Reassembly.Part(request.Disc.Number, part.LogicalPath, part.Offset, part.Size)
		if !ok {
			return fmt.Errorf("missing reassembly metadata for %q at offset %d", part.LogicalPath, part.Offset)
		}
		if err := writePart(ctx, writer, part, entry); err != nil {
			return err
		}
	}
	content, err := json.Marshal(request.Reassembly)
	if err != nil {
		return fmt.Errorf("marshal reassembly manifest: %w", err)
	}
	if err := writer.WriteHeader(&tar.Header{Name: reassemblyManifestPath, Mode: 0o600, Size: int64(len(content)), Format: tar.FormatPAX}); err != nil {
		return fmt.Errorf("write reassembly manifest header: %w", err)
	}
	if _, err := writer.Write(content); err != nil {
		return fmt.Errorf("write reassembly manifest: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close PAX tar archive: %w", err)
	}
	return nil
}

// VerifyPAXArchive validates a complete PAX tar stream and its reassembly manifest.
func VerifyPAXArchive(input io.Reader) error {
	reader := tar.NewReader(input)
	seen := make(map[string]struct{})
	hashes := make(map[string]string)
	var manifest ReassemblyManifest
	manifestFound := false
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read PAX tar entry: %w", err)
		}
		if !safeArchivePath(header.Name) {
			return fmt.Errorf("PAX tar entry has unsafe path %q", header.Name)
		}
		if _, exists := seen[header.Name]; exists {
			return fmt.Errorf("PAX tar contains duplicate entry %q", header.Name)
		}
		seen[header.Name] = struct{}{}
		if header.Name == reassemblyManifestPath {
			content, err := io.ReadAll(reader)
			if err != nil {
				return fmt.Errorf("read reassembly manifest: %w", err)
			}
			if err := json.Unmarshal(content, &manifest); err != nil {
				return fmt.Errorf("parse reassembly manifest: %w", err)
			}
			if manifest.FormatVersion != 1 || manifest.ArchiveSetID == "" {
				return fmt.Errorf("invalid reassembly manifest")
			}
			manifestFound = true
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeSymlink {
			return fmt.Errorf("PAX tar entry %q has unsupported type", header.Name)
		}
		if header.Typeflag == tar.TypeSymlink && (path.IsAbs(header.Linkname) || header.Linkname == "") {
			return fmt.Errorf("PAX tar symlink %q has unsafe target", header.Name)
		}
		hash := sha256.New()
		if _, err := io.Copy(hash, reader); err != nil {
			return fmt.Errorf("read PAX tar entry %q: %w", header.Name, err)
		}
		if header.Typeflag == tar.TypeReg {
			hashes[header.Name] = hex.EncodeToString(hash.Sum(nil))
		}
	}
	if !manifestFound {
		return fmt.Errorf("PAX tar is missing the reassembly manifest")
	}
	for entryName, hash := range hashes {
		matched := false
		for _, part := range manifest.Parts {
			if part.EntryName == entryName {
				matched = true
				if hash != part.SHA256 {
					return fmt.Errorf("PAX tar entry %q does not match its reassembly hash", entryName)
				}
			}
		}
		if !matched {
			return fmt.Errorf("PAX tar entry %q is not in the reassembly manifest", entryName)
		}
	}
	return nil
}

func writePart(ctx context.Context, writer *tar.Writer, part domain.Part, entry ReassemblyPart) error {
	file, err := os.Open(part.SourcePath)
	if err != nil {
		return fmt.Errorf("open source %q: %w", part.LogicalPath, err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat source %q: %w", part.LogicalPath, err)
	}
	if info.Size() != entry.OriginalSize {
		return fmt.Errorf("source %q changed since manifest hashing", part.LogicalPath)
	}
	if err := validatePartRange(part, info.Size()); err != nil {
		return err
	}
	if err := writer.WriteHeader(&tar.Header{Name: entry.EntryName, Mode: int64(info.Mode().Perm()), Size: part.Size, ModTime: info.ModTime(), Format: tar.FormatPAX}); err != nil {
		return fmt.Errorf("write PAX header for %q: %w", part.LogicalPath, err)
	}
	hash := sha256.New()
	reader := io.NewSectionReader(file, part.Offset, part.Size)
	written, err := io.Copy(writer, contextReader{ctx: ctx, reader: io.TeeReader(reader, hash)})
	if err != nil {
		return fmt.Errorf("write source range for %q: %w", part.LogicalPath, err)
	}
	if written != part.Size {
		return fmt.Errorf("source %q ended after %d bytes, expected %d", part.LogicalPath, written, part.Size)
	}
	observability.RecordIO(ctx, "pax.source_read", written)
	if got := hex.EncodeToString(hash.Sum(nil)); got != entry.SHA256 {
		return fmt.Errorf("source %q changed since manifest hashing", part.LogicalPath)
	}
	return nil
}

func writeSymlink(writer *tar.Writer, part domain.Part) error {
	target := relativeArchivePath(path.Dir(part.LogicalPath), part.SymlinkTarget)
	if err := writer.WriteHeader(&tar.Header{Name: part.LogicalPath, Linkname: target, Mode: 0o777, Typeflag: tar.TypeSymlink, Format: tar.FormatPAX}); err != nil {
		return fmt.Errorf("write symlink %q: %w", part.LogicalPath, err)
	}
	return nil
}

func relativeArchivePath(fromDirectory, target string) string {
	from := strings.Split(strings.Trim(fromDirectory, "/"), "/")
	to := strings.Split(strings.Trim(target, "/"), "/")
	if fromDirectory == "." {
		from = nil
	}
	common := 0
	for common < len(from) && common < len(to) && from[common] == to[common] {
		common++
	}
	relative := make([]string, 0, len(from)-common+len(to)-common)
	for range from[common:] {
		relative = append(relative, "..")
	}
	relative = append(relative, to[common:]...)
	if len(relative) == 0 {
		return "."
	}
	return strings.Join(relative, "/")
}

func safeArchivePath(value string) bool {
	return value != "" && !path.IsAbs(value) && value != "." && value != ".." && !strings.HasPrefix(value, "../")
}

func partEntryName(part domain.Part, originalSize int64) string {
	if part.Offset == 0 && part.Size == originalSize {
		return part.LogicalPath
	}
	id := sha256.Sum256([]byte(part.LogicalPath))
	return fmt.Sprintf(".archiver/parts/%x/%020d.part", id[:], part.Offset)
}

func validatePartRange(part domain.Part, originalSize int64) error {
	if part.SourcePath == "" || part.LogicalPath == "" || part.Offset < 0 || part.Size < 0 || part.Offset > originalSize || part.Size > originalSize-part.Offset {
		return fmt.Errorf("invalid planned range for source %q", part.LogicalPath)
	}
	return nil
}

func hashPart(ctx context.Context, part domain.Part) (string, error) {
	file, err := os.Open(part.SourcePath)
	if err != nil {
		return "", fmt.Errorf("open source %q: %w", part.LogicalPath, err)
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	written, err := io.Copy(hash, contextReader{ctx: ctx, reader: io.NewSectionReader(file, part.Offset, part.Size)})
	if err != nil {
		return "", fmt.Errorf("hash source range for %q: %w", part.LogicalPath, err)
	}
	if written != part.Size {
		return "", fmt.Errorf("source %q ended after %d bytes, expected %d", part.LogicalPath, written, part.Size)
	}
	observability.RecordIO(ctx, "source.hash", written)
	return hex.EncodeToString(hash.Sum(nil)), nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}
