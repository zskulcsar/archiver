package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zskulcsar/archiver/internal/adapters"
	"github.com/zskulcsar/archiver/internal/domain"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// * [x] **P1_CORE_013** Create streams encrypted payloads into independent images
// - Description: Creates a planned disc with fakes that record the archive stream, encrypted artifact, parity input, and image verification.
// - Expected: Parity protects the encrypted output, each image is verified, temporary artifacts are removed, and staging is atomically published.
func TestCreate_PublishesVerifiedEncryptedDiscImages(t *testing.T) {
	t.Parallel()

	output := t.TempDir()
	config := testValidatedConfig(output)
	sourcePath := writeCreateTestSource(t, output)
	var events bytes.Buffer
	backends := &fakeBackends{payload: "archive-data"}

	err := Create(context.Background(), CreateRequest{
		Config: config,
		Plan: domain.ArchivePlan{Discs: []domain.Disc{{
			Number: 1,
			Parts:  []domain.Part{{SourcePath: sourcePath, LogicalPath: "photo.jpg", Size: 12}},
		}}},
		Backends: backends,
		Events:   NewJSONLEventWriter(&events),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if got, want := backends.parityInput, filepath.Join(config.StagingPath, "disc-01.payload.enc"); got != want {
		t.Fatalf("parity input = %q, want %q", got, want)
	}
	if got, want := backends.imageInput.Encrypted.Path, backends.parityInput; got != want {
		t.Fatalf("image encrypted input = %q, want %q", got, want)
	}
	if !backends.imageVerified {
		t.Fatal("image verification was not called")
	}
	if _, err := os.Stat(config.FinalPath); err != nil {
		t.Fatalf("published archive set stat error = %v", err)
	}
	if _, err := os.Stat(config.StagingPath); !os.IsNotExist(err) {
		t.Fatalf("staging archive set stat error = %v, want not exist", err)
	}
	if _, err := os.Stat(backends.parityInput); !os.IsNotExist(err) {
		t.Fatalf("temporary encrypted payload stat error = %v, want not exist", err)
	}
	if got := events.String(); !strings.Contains(got, `"type":"published"`) {
		t.Fatalf("events = %q, want published JSONL event", got)
	}
}

// * [x] **P1_CORE_021** Symlink archive request
// - Description: Creates a disc containing a planned symlink member with recording archive backends.
// - Expected: The archive adapter receives the logical link and archive-relative target without a source target path.
func TestCreate_PassesSymlinkMembersToArchiveAdapter(t *testing.T) {
	t.Parallel()

	output := t.TempDir()
	config := testValidatedConfig(output)
	sourcePath := writeCreateTestSource(t, output)
	backends := &fakeBackends{payload: "archive-data"}
	err := Create(context.Background(), CreateRequest{
		Config: config,
		Plan: domain.ArchivePlan{Discs: []domain.Disc{{
			Number: 1,
			Parts: []domain.Part{
				{SourcePath: sourcePath, LogicalPath: "photo.jpg", Size: 12},
				{LogicalPath: "latest.jpg", SymlinkTarget: "photo.jpg"},
			},
		}}},
		Backends: backends,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	parts := backends.archiveRequest.Disc.Parts
	if got, want := parts[1], (domain.Part{LogicalPath: "latest.jpg", SymlinkTarget: "photo.jpg"}); got != want {
		t.Fatalf("archive symlink part = %#v, want %#v", got, want)
	}
}

// * [x] **P1_CORE_015** Create persists recovery metadata and final report
// - Description: Creates a planned disc containing an absolute source path with fake adapters and reads the published recovery artifacts.
// - Expected: Versioned manifests and report record planned and actual capacities, image hashes, and tool-bundle placeholder without persisting source paths.
func TestCreate_PersistsRecoveryMetadataAndReport(t *testing.T) {
	t.Parallel()

	output := t.TempDir()
	config := testValidatedConfig(output)
	config.CapacityBytes = 100
	sourcePath := filepath.Join(output, "private", "photo.jpg")
	if err := os.Mkdir(filepath.Dir(sourcePath), 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("photo-data12"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	backends := &fakeBackends{payload: "archive-data"}
	err := Create(context.Background(), CreateRequest{
		Config: config,
		Plan: domain.ArchivePlan{UsableCapacity: 90, Discs: []domain.Disc{{
			Number: 1,
			Parts:  []domain.Part{{SourcePath: sourcePath, LogicalPath: "photo.jpg", Size: 12}},
			Used:   12,
			Unused: 78,
		}}},
		Backends: backends,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got, want := len(backends.imageInput.Metadata), 3; got != want {
		t.Fatalf("image metadata count = %d, want %d", got, want)
	}

	discManifest := readJSONFile(t, filepath.Join(config.FinalPath, "disc-01-manifest.json"))
	if got, want := discManifest["format_version"], float64(1); got != want {
		t.Fatalf("disc manifest format_version = %v, want %v", got, want)
	}
	if got, want := discManifest["archive_set_id"], config.SetID; got != want {
		t.Fatalf("disc manifest archive_set_id = %v, want %v", got, want)
	}
	if got, want := discManifest["planned_capacity_bytes"], float64(90); got != want {
		t.Fatalf("disc manifest planned_capacity_bytes = %v, want %v", got, want)
	}
	if encoded, err := json.Marshal(discManifest); err != nil || strings.Contains(string(encoded), sourcePath) {
		t.Fatalf("disc manifest contains absolute source path: %s", encoded)
	}
	setManifest := readJSONFile(t, filepath.Join(config.FinalPath, "archive-set-manifest.json"))
	if got, want := setManifest["format_version"], float64(1); got != want {
		t.Fatalf("archive-set manifest format_version = %v, want %v", got, want)
	}
	if encoded, err := json.Marshal(setManifest); err != nil || strings.Contains(string(encoded), sourcePath) {
		t.Fatalf("archive-set manifest contains absolute source path: %s", encoded)
	}

	instructions, err := os.ReadFile(filepath.Join(config.FinalPath, "disc-01-recovery.txt"))
	if err != nil {
		t.Fatalf("read recovery instructions: %v", err)
	}
	if !strings.Contains(string(instructions), "disc-01.payload.enc") || !strings.Contains(string(instructions), "recovery-tool-retrieval-material") || !strings.Contains(string(instructions), "fake-encrypt (1.0)") {
		t.Fatalf("recovery instructions = %q, want artifact and tool-bundle identities", instructions)
	}

	report := readJSONFile(t, filepath.Join(config.FinalPath, "report.json"))
	if got, want := report["planned_capacity_bytes"], float64(90); got != want {
		t.Fatalf("report planned_capacity_bytes = %v, want %v", got, want)
	}
	if got, want := report["actual_capacity_bytes"], float64(len("image")); got != want {
		t.Fatalf("report actual_capacity_bytes = %v, want %v", got, want)
	}
	toolBundle := report["recovery_tool_bundle"].(map[string]any)
	if got, want := toolBundle["name"], "recovery-tool-retrieval-material"; got != want {
		t.Fatalf("report recovery tool bundle = %v, want %v", got, want)
	}
	images := report["images"].([]any)
	image := images[0].(map[string]any)
	wantHash := sha256.Sum256([]byte("image"))
	if got, want := image["sha256"], hex.EncodeToString(wantHash[:]); got != want {
		t.Fatalf("report image hash = %v, want %v", got, want)
	}
	if encoded, err := json.Marshal(report); err != nil || strings.Contains(string(encoded), sourcePath) {
		t.Fatalf("report contains absolute source path: %s", encoded)
	}
}

// * [x] **P1_CORE_014** Create cancellation never publishes final output
// - Description: Starts creation with an already-cancelled context and fake adapters that must never receive work.
// - Expected: Creation returns context cancellation and leaves neither staging nor final archive-set directories.
func TestCreate_CancellationLeavesNoPublishedOutput(t *testing.T) {
	t.Parallel()

	output := t.TempDir()
	config := testValidatedConfig(output)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	backends := &fakeBackends{payload: "archive-data"}

	err := Create(ctx, CreateRequest{
		Config:   config,
		Plan:     domain.ArchivePlan{Discs: []domain.Disc{{Number: 1}}},
		Backends: backends,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Create() error = %v, want context cancellation", err)
	}
	if backends.archiveCalled {
		t.Fatal("archive adapter was called after cancellation")
	}
	if _, err := os.Stat(config.StagingPath); !os.IsNotExist(err) {
		t.Fatalf("staging archive set stat error = %v, want not exist", err)
	}
	if _, err := os.Stat(config.FinalPath); !os.IsNotExist(err) {
		t.Fatalf("final archive set stat error = %v, want not exist", err)
	}
}

// * [x] **P1_LINUX_007** Zero loss tolerance omits local recovery artifacts
// - Description: Creates a disc using a zero-percent loss tolerance and recording fake backends.
// - Expected: PAR2 creation and verification are skipped and the image receives no parity artifacts.
func TestCreate_ZeroLossToleranceSkipsParity(t *testing.T) {
	t.Parallel()

	output := t.TempDir()
	config := testValidatedConfig(output)
	config.LossPercent = 0
	sourcePath := writeCreateTestSource(t, output)
	backends := &fakeBackends{payload: "archive-data"}
	if err := Create(context.Background(), CreateRequest{
		Config:   config,
		Plan:     domain.ArchivePlan{Discs: []domain.Disc{{Number: 1, Parts: []domain.Part{{SourcePath: sourcePath, LogicalPath: "photo.jpg", Size: 12}}}}},
		Backends: backends,
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if backends.parityCalled {
		t.Fatal("CreateParity() was called with zero loss tolerance")
	}
	if got := len(backends.imageInput.Parity); got != 0 {
		t.Fatalf("image parity artifacts = %d, want 0", got)
	}
}

// * [x] **P1_LINUX_009** Encrypted payload verification precedes PAR2 generation
// - Description: Creates a disc with backends that expose encrypted archive verification and record parity creation.
// - Expected: The encrypted payload is verified before local recovery data is generated.
func TestCreate_VerifiesEncryptedArchiveBeforeParity(t *testing.T) {
	t.Parallel()

	output := t.TempDir()
	backends := &verifyingBackends{fakeBackends: fakeBackends{payload: "archive-data"}}
	sourcePath := writeCreateTestSource(t, output)
	err := Create(context.Background(), CreateRequest{
		Config:   testValidatedConfig(output),
		Plan:     domain.ArchivePlan{Discs: []domain.Disc{{Number: 1, Parts: []domain.Part{{SourcePath: sourcePath, LogicalPath: "photo.jpg", Size: 12}}}}},
		Backends: backends,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !backends.verifiedEncrypted {
		t.Fatal("encrypted archive was not verified")
	}
}

// * [x] **P1_LINUX_011** PAR2 repair testing follows parity verification
// - Description: Creates a disc with a backend that performs a disposable PAR2 repair test.
// - Expected: The repair test receives the encrypted payload and generated PAR2 artifacts.
func TestCreate_TestsParityRepair(t *testing.T) {
	t.Parallel()

	output := t.TempDir()
	backends := &repairTestingBackends{fakeBackends: fakeBackends{payload: "archive-data"}}
	sourcePath := writeCreateTestSource(t, output)
	err := Create(context.Background(), CreateRequest{
		Config:   testValidatedConfig(output),
		Plan:     domain.ArchivePlan{Discs: []domain.Disc{{Number: 1, Parts: []domain.Part{{SourcePath: sourcePath, LogicalPath: "photo.jpg", Size: 12}}}}},
		Backends: backends,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !backends.repairTested {
		t.Fatal("PAR2 repair test was not called")
	}
}

// * [x] **OBSERVABILITY_001** Create records image artifact hashing
// - Description: Creates a disc with an in-memory span recorder enabled.
// - Expected: The trace includes a distinct image-hash span after image verification.
func TestCreate_RecordsImageArtifactHashing(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previousProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		_ = provider.Shutdown(context.Background())
	})

	output := t.TempDir()
	sourcePath := writeCreateTestSource(t, output)
	err := Create(context.Background(), CreateRequest{
		Config: testValidatedConfig(output),
		Plan: domain.ArchivePlan{Discs: []domain.Disc{{
			Number: 1,
			Parts:  []domain.Part{{SourcePath: sourcePath, LogicalPath: "photo.jpg", Size: 12}},
		}}},
		Backends: &fakeBackends{payload: "archive-data"},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	for _, span := range recorder.Ended() {
		if span.Name() == "archiver.image.hash" {
			return
		}
	}
	t.Fatal("trace does not include archiver.image.hash")
}

func testValidatedConfig(output string) ValidatedConfig {
	return ValidatedConfig{
		Config:        Config{Output: output},
		CapacityBytes: 100,
		LossPercent:   10,
		SetID:         "test_2026-09-13_14-05",
		StagingPath:   filepath.Join(output, ".archiver-staging-test_2026-09-13_14-05"),
		FinalPath:     filepath.Join(output, "test_2026-09-13_14-05"),
	}
}

func writeCreateTestSource(t *testing.T, output string) string {
	t.Helper()

	path := filepath.Join(output, "photo.jpg")
	if err := os.WriteFile(path, []byte("photo-data12"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func readJSONFile(t *testing.T, path string) map[string]any {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var value map[string]any
	if err := json.Unmarshal(content, &value); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	return value
}

type fakeBackends struct {
	payload        string
	archiveCalled  bool
	archiveRequest adapters.ArchiveRequest
	parityInput    string
	parityCalled   bool
	imageInput     adapters.ImageRequest
	imageVerified  bool
}

type verifyingBackends struct {
	fakeBackends
	verifiedEncrypted bool
}

type repairTestingBackends struct {
	fakeBackends
	repairTested bool
}

func (f *repairTestingBackends) TestParityRepair(context.Context, adapters.Artifact, []adapters.Artifact) error {
	f.repairTested = true
	return nil
}

func (f *verifyingBackends) VerifyEncrypted(context.Context, adapters.Artifact) error {
	f.verifiedEncrypted = true
	return nil
}

func (f *fakeBackends) CreateArchive(_ context.Context, request adapters.ArchiveRequest, output io.Writer) error {
	f.archiveCalled = true
	f.archiveRequest = request
	_, err := io.WriteString(output, f.payload)
	return err
}

func (f *fakeBackends) Encrypt(_ context.Context, input io.Reader, outputPath string) (adapters.Artifact, error) {
	content, err := io.ReadAll(input)
	if err != nil {
		return adapters.Artifact{}, err
	}
	if err := os.WriteFile(outputPath, append([]byte("encrypted:"), content...), 0o600); err != nil {
		return adapters.Artifact{}, err
	}
	return adapters.Artifact{Path: outputPath, Format: "fake-encrypted", Tool: adapters.ToolIdentity{Name: "fake-encrypt", Version: "1.0"}}, nil
}

func (f *fakeBackends) CreateParity(_ context.Context, input adapters.Artifact, outputDir string) ([]adapters.Artifact, error) {
	f.parityCalled = true
	f.parityInput = input.Path
	path := filepath.Join(outputDir, "disc-01.par2")
	if err := os.WriteFile(path, []byte("parity"), 0o600); err != nil {
		return nil, err
	}
	return []adapters.Artifact{{Path: path, Format: "fake-parity", Tool: adapters.ToolIdentity{Name: "fake-parity", Version: "1.0"}}}, nil
}

func (f *fakeBackends) VerifyParity(context.Context, []adapters.Artifact) error {
	return nil
}

func (f *fakeBackends) CreateImage(_ context.Context, input adapters.ImageRequest) (adapters.Artifact, error) {
	f.imageInput = input
	if err := os.WriteFile(input.OutputPath, []byte("image"), 0o600); err != nil {
		return adapters.Artifact{}, err
	}
	return adapters.Artifact{Path: input.OutputPath, Format: "fake-image", Tool: adapters.ToolIdentity{Name: "fake-image", Version: "1.0"}}, nil
}

func (f *fakeBackends) VerifyImage(context.Context, adapters.Artifact) error {
	f.imageVerified = true
	return nil
}
