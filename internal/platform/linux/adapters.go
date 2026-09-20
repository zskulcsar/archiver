package linux

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/zskulcsar/archiver/internal/adapters"
	"github.com/zskulcsar/archiver/internal/observability"
	"go.opentelemetry.io/otel/attribute"
)

// Backends implements image-only creation with a portable PAX tar creator and Linux system tools.
type Backends struct {
	adapters.PAXTarCreator
	tools          Tools
	passphrasePath string
	lossPercent    int
}

// NewBackends creates image-only Linux adapters from discovered tools.
func NewBackends(tools Tools, passphrasePath string, lossPercent int) *Backends {
	return &Backends{tools: tools, passphrasePath: passphrasePath, lossPercent: lossPercent}
}

// Encrypt symmetrically encrypts input with GnuPG AES-256 using file descriptor 3 for the passphrase.
func (b *Backends) Encrypt(ctx context.Context, input io.Reader, outputPath string) (artifact adapters.Artifact, err error) {
	ctx, span := observability.Start(ctx, "archiver.gpg.encrypt")
	defer func() {
		observability.RecordError(span, err)
		span.End()
	}()
	passphrase, err := os.Open(b.passphrasePath)
	if err != nil {
		return adapters.Artifact{}, fmt.Errorf("open passphrase file: %w", err)
	}
	defer func() { _ = passphrase.Close() }()

	command := exec.CommandContext(ctx, b.tools.GPG.Path, gpgArguments(outputPath)...)
	command.Stdin = input
	command.ExtraFiles = []*os.File{passphrase}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return adapters.Artifact{}, fmt.Errorf("encrypt archive: %w: %s", err, stderr.String())
	}
	artifact = artifactForPath(outputPath, "application/pgp-encrypted", b.tools.GPG)
	observability.RecordIO(ctx, "gpg.encrypted_output", artifact.Size)
	return artifact, nil
}

func gpgArguments(outputPath string) []string {
	return []string{"--batch", "--yes", "--pinentry-mode", "loopback", "--passphrase-fd", "3", "--symmetric", "--cipher-algo", "AES256", "--output", outputPath}
}

// VerifyEncrypted decrypts an encrypted payload and validates its PAX tar contents.
func (b *Backends) VerifyEncrypted(ctx context.Context, encrypted adapters.Artifact) (err error) {
	ctx, span := observability.Start(ctx, "archiver.gpg.decrypt_verify")
	defer func() {
		observability.RecordError(span, err)
		span.End()
	}()
	passphrase, err := os.Open(b.passphrasePath)
	if err != nil {
		return fmt.Errorf("open passphrase file: %w", err)
	}
	defer func() { _ = passphrase.Close() }()

	decrypt := exec.CommandContext(ctx, b.tools.GPG.Path, "--batch", "--yes", "--pinentry-mode", "loopback", "--passphrase-fd", "3", "--decrypt", encrypted.Path)
	decrypt.ExtraFiles = []*os.File{passphrase}
	output, err := decrypt.StdoutPipe()
	if err != nil {
		return fmt.Errorf("open decrypted archive stream: %w", err)
	}
	var decryptErr bytes.Buffer
	decrypt.Stderr = &decryptErr
	if err := decrypt.Start(); err != nil {
		return fmt.Errorf("start decrypt encrypted payload: %w", err)
	}
	verifyErr := adapters.VerifyPAXArchive(output)
	decryptRunErr := decrypt.Wait()
	if decryptRunErr != nil {
		return fmt.Errorf("decrypt encrypted payload: %w: %s", decryptRunErr, decryptErr.String())
	}
	if verifyErr != nil {
		return fmt.Errorf("verify decrypted PAX tar: %w", verifyErr)
	}
	return nil
}

// CreateParity creates local PAR2 recovery files for an encrypted payload.
func (b *Backends) CreateParity(ctx context.Context, input adapters.Artifact, outputDir string) (artifacts []adapters.Artifact, err error) {
	ctx, span := observability.Start(ctx, "archiver.par2.create", attribute.Int("archiver.loss_percent", b.lossPercent))
	defer func() {
		observability.RecordError(span, err)
		span.End()
	}()
	prefix := filepath.Join(outputDir, filepath.Base(input.Path))
	command := exec.CommandContext(ctx, b.tools.PAR2.Path, par2CreateArguments(b.lossPercent, prefix, input.Path)...)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("create PAR2 recovery data: %w: %s", err, stderr.String())
	}
	paths, err := filepath.Glob(prefix + "*.par2")
	if err != nil {
		return nil, fmt.Errorf("list PAR2 recovery data: %w", err)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("PAR2 did not create recovery data")
	}
	sort.Strings(paths)
	artifacts = make([]adapters.Artifact, 0, len(paths))
	for _, path := range paths {
		artifacts = append(artifacts, artifactForPath(path, "application/par2", b.tools.PAR2))
	}
	return artifacts, nil
}

func par2CreateArguments(lossPercent int, prefix, inputPath string) []string {
	return []string{"create", fmt.Sprintf("-r%d", lossPercent), "-n1", prefix, inputPath}
}

// VerifyParity verifies a PAR2 recovery set.
func (b *Backends) VerifyParity(ctx context.Context, artifacts []adapters.Artifact) (err error) {
	ctx, span := observability.Start(ctx, "archiver.par2.verify")
	defer func() {
		observability.RecordError(span, err)
		span.End()
	}()
	if len(artifacts) == 0 {
		return nil
	}
	command := exec.CommandContext(ctx, b.tools.PAR2.Path, "verify", artifacts[0].Path)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("verify PAR2 recovery data: %w: %s", err, stderr.String())
	}
	return nil
}

// TestParityRepair corrupts a disposable encrypted-payload copy and confirms PAR2 repairs it.
func (b *Backends) TestParityRepair(ctx context.Context, encrypted adapters.Artifact, artifacts []adapters.Artifact) (err error) {
	ctx, span := observability.Start(ctx, "archiver.par2.repair_test")
	defer func() {
		observability.RecordError(span, err)
		span.End()
	}()
	if len(artifacts) == 0 {
		return nil
	}
	workspace, err := os.MkdirTemp(filepath.Dir(encrypted.Path), ".archiver-par2-repair-")
	if err != nil {
		return fmt.Errorf("create PAR2 repair workspace: %w", err)
	}
	defer func() { _ = os.RemoveAll(workspace) }()

	copyPath := filepath.Join(workspace, filepath.Base(encrypted.Path))
	copyCtx, copySpan := observability.Start(ctx, "archiver.par2.repair_test.workspace_copy")
	copyErr := func() error {
		if err := copyFile(encrypted.Path, copyPath); err != nil {
			return fmt.Errorf("copy encrypted payload for repair test: %w", err)
		}
		observability.RecordIO(copyCtx, "par2.repair_test.payload_copy_read", encrypted.Size)
		observability.RecordIO(copyCtx, "par2.repair_test.payload_copy_write", encrypted.Size)
		for _, artifact := range artifacts {
			if err := copyFile(artifact.Path, filepath.Join(workspace, filepath.Base(artifact.Path))); err != nil {
				return fmt.Errorf("copy PAR2 artifact for repair test: %w", err)
			}
			observability.RecordIO(copyCtx, "par2.repair_test.parity_copy_read", artifact.Size)
			observability.RecordIO(copyCtx, "par2.repair_test.parity_copy_write", artifact.Size)
		}
		return nil
	}()
	observability.RecordError(copySpan, copyErr)
	copySpan.End()
	if copyErr != nil {
		return copyErr
	}
	if err := corruptFile(copyPath); err != nil {
		return fmt.Errorf("corrupt disposable encrypted payload: %w", err)
	}
	repairCtx, repairSpan := observability.Start(ctx, "archiver.par2.repair_test.repair")
	command := exec.CommandContext(repairCtx, b.tools.PAR2.Path, "repair", copyPath)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	repairErr := command.Run()
	observability.RecordError(repairSpan, repairErr)
	repairSpan.End()
	if repairErr != nil {
		return fmt.Errorf("repair disposable encrypted payload: %w: %s", repairErr, stderr.String())
	}
	compareCtx, compareSpan := observability.Start(ctx, "archiver.par2.repair_test.compare")
	match, compareErr := filesMatch(encrypted.Path, copyPath)
	if compareErr == nil {
		observability.RecordIO(compareCtx, "par2.repair_test.compare_read", encrypted.Size*2)
	}
	observability.RecordError(compareSpan, compareErr)
	compareSpan.End()
	if compareErr != nil {
		return fmt.Errorf("compare repaired encrypted payload: %w", compareErr)
	}
	if !match {
		return fmt.Errorf("PAR2 repair did not restore the encrypted payload")
	}
	return nil
}

// CreateImage creates a UDF ISO image containing only the current disc artifacts.
func (b *Backends) CreateImage(ctx context.Context, request adapters.ImageRequest) (artifact adapters.Artifact, err error) {
	ctx, span := observability.Start(ctx, "archiver.xorriso.image_create", attribute.Int("archiver.disc.number", request.DiscNumber))
	defer func() {
		observability.RecordError(span, err)
		span.End()
	}()
	arguments := []string{"-as", "mkisofs", "-iso-level", "3", "-o", request.OutputPath, "-graft-points"}
	arguments = append(arguments, filepath.Base(request.Encrypted.Path)+"="+request.Encrypted.Path)
	for _, parity := range request.Parity {
		arguments = append(arguments, filepath.Base(parity.Path)+"="+parity.Path)
	}
	for _, metadata := range request.Metadata {
		arguments = append(arguments, filepath.Base(metadata.Path)+"="+metadata.Path)
	}
	command := exec.CommandContext(ctx, b.tools.Xorriso.Path, arguments...)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return adapters.Artifact{}, fmt.Errorf("create UDF image: %w: %s", err, stderr.String())
	}
	artifact = artifactForPath(request.OutputPath, "application/x-iso9660-image", b.tools.Xorriso)
	observability.RecordIO(ctx, "xorriso.image_output", artifact.Size)
	return artifact, nil
}

// VerifyImage verifies that an image has a readable, non-empty filesystem structure.
func (b *Backends) VerifyImage(ctx context.Context, image adapters.Artifact) (err error) {
	ctx, span := observability.Start(ctx, "archiver.xorriso.image_verify")
	defer func() {
		observability.RecordError(span, err)
		span.End()
	}()
	command := exec.CommandContext(ctx, b.tools.Xorriso.Path, "-indev", image.Path, "-find", "/", "-type", "f", "-exec", "lsdl", "--")
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("verify image: %w: %s", err, stderr.String())
	}
	return nil
}

func artifactForPath(path, format string, tool Tool) adapters.Artifact {
	info, err := os.Stat(path)
	if err != nil {
		return adapters.Artifact{Path: path, Format: format, Tool: adapters.ToolIdentity{Name: tool.Name, Version: tool.Version, Path: tool.Path}}
	}
	return adapters.Artifact{Path: path, Size: info.Size(), Format: format, Tool: adapters.ToolIdentity{Name: tool.Name, Version: tool.Version, Path: tool.Path}}
}

func copyFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { _ = input.Close() }()
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func corruptFile(path string) error {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		return fmt.Errorf("cannot corrupt an empty payload")
	}
	offset := info.Size() / 2
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	var value [1]byte
	if _, err := io.ReadFull(file, value[:]); err != nil {
		return err
	}
	value[0] ^= 0xFF
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	_, err = file.Write(value[:])
	return err
}

func filesMatch(first, second string) (bool, error) {
	firstFile, err := os.Open(first)
	if err != nil {
		return false, err
	}
	defer func() { _ = firstFile.Close() }()
	secondFile, err := os.Open(second)
	if err != nil {
		return false, err
	}
	defer func() { _ = secondFile.Close() }()
	firstHash := sha256.New()
	secondHash := sha256.New()
	if _, err := io.Copy(firstHash, firstFile); err != nil {
		return false, err
	}
	if _, err := io.Copy(secondHash, secondFile); err != nil {
		return false, err
	}
	return bytes.Equal(firstHash.Sum(nil), secondHash.Sum(nil)), nil
}
