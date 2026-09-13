package adapters

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/zskulcsar/archiver/internal/domain"
)

// * [x] **P1_CORE_005** Versioned SQLite manifest round trip
// - Description: Saves and loads source records through the local SQLite manifest format.
// - Expected: The schema version and complete source records survive a database round trip.
func TestManifestStore_RoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "manifest.sqlite")
	store, err := OpenManifest(path)
	if err != nil {
		t.Fatalf("OpenManifest() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	want := []domain.Source{
		{SourcePath: "/source/a.txt", LogicalPath: "a.txt", Size: 12},
		{SourcePath: "/source/b.txt", LogicalPath: "dir/b.txt", Size: 34},
	}
	if err := store.ReplaceSources(ctx, want); err != nil {
		t.Fatalf("ReplaceSources() error = %v", err)
	}

	got, err := store.Sources(ctx)
	if err != nil {
		t.Fatalf("Sources() error = %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("len(Sources()) = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Sources()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}
