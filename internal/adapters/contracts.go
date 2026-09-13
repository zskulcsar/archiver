package adapters

import (
	"context"
	"io"

	"github.com/zskulcsar/archiver/internal/domain"
)

// Artifact identifies an adapter-created file.
type Artifact struct {
	Path   string
	Size   int64
	SHA256 string
	Format string
	Tool   ToolIdentity
}

// ToolIdentity identifies the tool that produced or consumes an artifact.
type ToolIdentity struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ArchiveRequest identifies the independent payload to stream for one disc.
type ArchiveRequest struct {
	Disc     domain.Disc
	Symlinks []domain.Symlink
}

// ArchiveCreator streams one disc's archive payload to output.
type ArchiveCreator interface {
	CreateArchive(ctx context.Context, request ArchiveRequest, output io.Writer) error
}

// Encryptor consumes an archive stream and creates its encrypted artifact at outputPath.
type Encryptor interface {
	Encrypt(ctx context.Context, input io.Reader, outputPath string) (Artifact, error)
}

// ParityCreator creates and verifies local recovery data for encrypted artifacts.
type ParityCreator interface {
	CreateParity(ctx context.Context, input Artifact, outputDir string) ([]Artifact, error)
	VerifyParity(ctx context.Context, artifacts []Artifact) error
}

// ImageRequest identifies the verified encrypted disc layout to encode as an image.
type ImageRequest struct {
	DiscNumber int
	Encrypted  Artifact
	Parity     []Artifact
	Metadata   []Artifact
	OutputPath string
}

// ImageCreator creates and verifies independently recoverable disc images.
type ImageCreator interface {
	CreateImage(ctx context.Context, request ImageRequest) (Artifact, error)
	VerifyImage(ctx context.Context, image Artifact) error
}
