package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/zskulcsar/archiver/internal/adapters"
	"github.com/zskulcsar/archiver/internal/domain"
)

// Event is a stable, privacy-preserving create lifecycle event.
type Event struct {
	Type string `json:"type"`
	Disc int    `json:"disc,omitempty"`
}

// EventSink receives create lifecycle events.
type EventSink interface {
	Emit(Event) error
}

// JSONLEventWriter writes one Event object per line.
type JSONLEventWriter struct {
	encoder *json.Encoder
}

// NewJSONLEventWriter creates an EventSink that writes JSON Lines to output.
func NewJSONLEventWriter(output io.Writer) *JSONLEventWriter {
	return &JSONLEventWriter{encoder: json.NewEncoder(output)}
}

// Emit writes event as a single JSON Line.
func (w *JSONLEventWriter) Emit(event Event) error {
	return w.encoder.Encode(event)
}

// CreateBackends contains the portable capabilities required by create.
type CreateBackends interface {
	adapters.ArchiveCreator
	adapters.Encryptor
	adapters.ParityCreator
	adapters.ImageCreator
}

// CreateRequest contains the validated allocation and tool contracts for an archive set.
type CreateRequest struct {
	Config      ValidatedConfig
	Plan        domain.ArchivePlan
	Backends    CreateBackends
	Events      EventSink
	DiskSpace   DiskSpaceChecker
	KeepStaging bool
}

// Create builds verified disc images in staging then atomically publishes the archive set.
func Create(ctx context.Context, request CreateRequest) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if request.Backends == nil {
		return fmt.Errorf("create backends are required")
	}
	if err := PreflightOutput(request.Config); err != nil {
		return err
	}
	if err := PreflightCapacity(request.Config, request.Plan, request.DiskSpace); err != nil {
		return err
	}
	if err := os.Mkdir(request.Config.StagingPath, 0o700); err != nil {
		return fmt.Errorf("create staging directory: %w", err)
	}
	defer func() {
		if err != nil && !request.KeepStaging {
			_ = os.RemoveAll(request.Config.StagingPath)
		}
	}()

	if err := emit(request.Events, Event{Type: "staging_started"}); err != nil {
		return err
	}
	results := make([]discResult, 0, len(request.Plan.Discs))
	for _, disc := range request.Plan.Discs {
		if err := ctx.Err(); err != nil {
			return err
		}
		result, err := createDisc(ctx, request, disc)
		if err != nil {
			return err
		}
		results = append(results, result)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := writeArchiveSetMetadata(request.Config.StagingPath, request.Config.SetID, request.Plan, results); err != nil {
		return err
	}
	if err := writeFinalReport(request.Config.StagingPath, request.Config.SetID, request.Plan, results); err != nil {
		return err
	}
	if err := os.Rename(request.Config.StagingPath, request.Config.FinalPath); err != nil {
		return fmt.Errorf("publish archive set: %w", err)
	}
	if err := emit(request.Events, Event{Type: "published"}); err != nil {
		return err
	}

	return nil
}

func createDisc(ctx context.Context, request CreateRequest, disc domain.Disc) (discResult, error) {
	if err := emit(request.Events, Event{Type: "archive_started", Disc: disc.Number}); err != nil {
		return discResult{}, err
	}
	payloadPath := filepath.Join(request.Config.StagingPath, fmt.Sprintf("disc-%02d.payload.enc", disc.Number))
	symlinks := make([]domain.Symlink, 0)
	for _, part := range disc.Parts {
		if part.SymlinkTarget != "" {
			symlinks = append(symlinks, domain.Symlink{LogicalPath: part.LogicalPath, TargetPath: part.SymlinkTarget})
		}
	}
	encrypted, err := streamEncrypt(ctx, request.Backends, adapters.ArchiveRequest{Disc: disc, Symlinks: symlinks}, payloadPath)
	if err != nil {
		return discResult{}, fmt.Errorf("create encrypted payload for disc %d: %w", disc.Number, err)
	}
	if err := emit(request.Events, Event{Type: "archive_completed", Disc: disc.Number}); err != nil {
		return discResult{}, err
	}
	if err := emit(request.Events, Event{Type: "parity_started", Disc: disc.Number}); err != nil {
		return discResult{}, err
	}
	parity, err := request.Backends.CreateParity(ctx, encrypted, request.Config.StagingPath)
	if err != nil {
		return discResult{}, fmt.Errorf("create parity for disc %d: %w", disc.Number, err)
	}
	if err := request.Backends.VerifyParity(ctx, parity); err != nil {
		return discResult{}, VerificationError{Err: fmt.Errorf("verify parity for disc %d: %w", disc.Number, err)}
	}
	if err := emit(request.Events, Event{Type: "parity_completed", Disc: disc.Number}); err != nil {
		return discResult{}, err
	}
	artifacts := make([]artifactMetadata, 0, len(parity)+1)
	payloadMetadata, err := metadataForArtifact(encrypted, "encrypted_payload")
	if err != nil {
		return discResult{}, err
	}
	artifacts = append(artifacts, payloadMetadata)
	for _, parityArtifact := range parity {
		metadata, err := metadataForArtifact(parityArtifact, "parity")
		if err != nil {
			return discResult{}, err
		}
		artifacts = append(artifacts, metadata)
	}
	manifestName, metadata, err := writeDiscMetadata(request.Config.StagingPath, request.Config.SetID, request.Plan.UsableCapacity, disc, artifacts)
	if err != nil {
		return discResult{}, err
	}
	imagePath := filepath.Join(request.Config.StagingPath, fmt.Sprintf("disc-%02d.img", disc.Number))
	if err := emit(request.Events, Event{Type: "image_started", Disc: disc.Number}); err != nil {
		return discResult{}, err
	}
	image, err := request.Backends.CreateImage(ctx, adapters.ImageRequest{
		DiscNumber: disc.Number,
		Encrypted:  encrypted,
		Parity:     parity,
		Metadata:   metadata,
		OutputPath: imagePath,
	})
	if err != nil {
		return discResult{}, fmt.Errorf("create image for disc %d: %w", disc.Number, err)
	}
	if err := request.Backends.VerifyImage(ctx, image); err != nil {
		return discResult{}, VerificationError{Err: fmt.Errorf("verify image for disc %d: %w", disc.Number, err)}
	}
	if err := emit(request.Events, Event{Type: "image_verified", Disc: disc.Number}); err != nil {
		return discResult{}, err
	}
	imageMetadata, err := metadataForArtifact(image, "image")
	if err != nil {
		return discResult{}, err
	}
	if err := os.Remove(encrypted.Path); err != nil {
		return discResult{}, fmt.Errorf("remove encrypted payload for disc %d: %w", disc.Number, err)
	}
	for _, artifact := range parity {
		if err := os.Remove(artifact.Path); err != nil {
			return discResult{}, fmt.Errorf("remove parity artifact for disc %d: %w", disc.Number, err)
		}
	}
	if err := emit(request.Events, Event{Type: "disc_completed", Disc: disc.Number}); err != nil {
		return discResult{}, err
	}
	tools := make([]adapters.ToolIdentity, 0, len(artifacts))
	for _, artifact := range artifacts {
		tools = appendTool(tools, artifact.Tool)
	}
	tools = appendTool(tools, imageMetadata.Tool)
	return discResult{discManifestName: manifestName, image: imageMetadata, tools: tools}, nil
}

func streamEncrypt(ctx context.Context, backends CreateBackends, archive adapters.ArchiveRequest, outputPath string) (adapters.Artifact, error) {
	reader, writer := io.Pipe()
	archiveErr := make(chan error, 1)
	go func() {
		err := backends.CreateArchive(ctx, archive, writer)
		if err != nil {
			_ = writer.CloseWithError(err)
		} else {
			_ = writer.Close()
		}
		archiveErr <- err
	}()
	encrypted, encryptionErr := backends.Encrypt(ctx, reader, outputPath)
	if encryptionErr != nil {
		_ = reader.CloseWithError(encryptionErr)
	}
	if err := <-archiveErr; err != nil {
		return adapters.Artifact{}, err
	}
	if err := ctx.Err(); err != nil {
		return adapters.Artifact{}, err
	}
	if encryptionErr != nil {
		return adapters.Artifact{}, encryptionErr
	}
	return encrypted, nil
}

func emit(events EventSink, event Event) error {
	if events == nil {
		return nil
	}
	if err := events.Emit(event); err != nil {
		return fmt.Errorf("write %s event: %w", event.Type, err)
	}
	return nil
}
