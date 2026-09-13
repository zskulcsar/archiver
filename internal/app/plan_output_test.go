package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zskulcsar/archiver/internal/domain"
)

// * [x] **P1_CORE_017** Persisted JSON archive plan
// - Description: Writes a deterministic plan artifact containing disc and file-part layout without source-machine paths.
// - Expected: The output parent receives a new JSON file with the set ID, discs, logical paths, offsets, and byte sizes.
func TestWritePlanJSON_WritesDiscLayout(t *testing.T) {
	t.Parallel()

	output := t.TempDir()
	config := testValidatedConfig(output)
	plan := domain.ArchivePlan{
		UsableCapacity: 2_000_000_000,
		Discs: []domain.Disc{{
			Number: 1,
			Used:   1_200,
			Unused: 1_999_998_800,
			Parts: []domain.Part{{
				LogicalPath: "photos/a.jpg",
				Offset:      0,
				Size:        1_200,
			}},
		}},
	}

	path, err := WritePlanJSON(config, plan)
	if err != nil {
		t.Fatalf("WritePlanJSON() error = %v", err)
	}
	if got, want := path, filepath.Join(output, config.SetID+".plan.json"); got != want {
		t.Fatalf("WritePlanJSON() path = %q, want %q", got, want)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var got struct {
		ArchiveSetID string `json:"archive_set_id"`
		Discs        []struct {
			Parts []struct {
				LogicalPath string `json:"logical_path"`
				Offset      int64  `json:"offset_bytes"`
				Size        int64  `json:"size_bytes"`
			} `json:"parts"`
		} `json:"discs"`
	}
	if err := json.Unmarshal(content, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got.ArchiveSetID != config.SetID || len(got.Discs) != 1 {
		t.Fatalf("plan JSON = %+v, want set %q with one disc", got, config.SetID)
	}
	part := got.Discs[0].Parts[0]
	if part.LogicalPath != "photos/a.jpg" || part.Offset != 0 || part.Size != 1_200 {
		t.Fatalf("plan part = %+v, want logical path, offset, and size", part)
	}
}

// * [x] **P1_CORE_021** Existing plan output has actionable remediation
// - Description: Refuses to overwrite an existing plan layout artifact.
// - Expected: The error states the stop, existing-file reason, and a corrective action.
func TestWritePlanJSON_RejectsExistingPlanFile(t *testing.T) {
	t.Parallel()

	output := t.TempDir()
	config := testValidatedConfig(output)
	path := filepath.Join(output, config.SetID+".plan.json")
	if err := os.WriteFile(path, []byte("existing"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := WritePlanJSON(config, domain.ArchivePlan{})
	if err == nil {
		t.Fatal("WritePlanJSON() error = nil, want error")
	}
	for _, want := range []string{"stopped before writing archive plan", "already exists", "remove or rename"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("WritePlanJSON() error = %q, want to contain %q", err, want)
		}
	}
}
