package adapters

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/zskulcsar/archiver/internal/domain"
	_ "modernc.org/sqlite"
)

const manifestFormatVersion = 1

// ManifestStore stores the local plaintext source manifest in SQLite.
type ManifestStore struct {
	db *sql.DB
}

// OpenManifest opens a versioned SQLite manifest at path.
func OpenManifest(path string) (*ManifestStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open manifest: %w", err)
	}

	store := &ManifestStore{db: db}
	if err := store.initialize(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

// Close closes the manifest database.
func (s *ManifestStore) Close() error {
	return s.db.Close()
}

// ReplaceSources atomically replaces the manifest source records.
func (s *ManifestStore) ReplaceSources(ctx context.Context, sources []domain.Source) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin source replacement: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM sources`); err != nil {
		return fmt.Errorf("delete manifest sources: %w", err)
	}
	statement, err := tx.PrepareContext(ctx, `INSERT INTO sources (source_path, logical_path, size_bytes) VALUES (?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare source insert: %w", err)
	}
	defer func() { _ = statement.Close() }()

	for _, source := range sources {
		if _, err := statement.ExecContext(ctx, source.SourcePath, source.LogicalPath, source.Size); err != nil {
			return fmt.Errorf("insert source %q: %w", source.LogicalPath, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit source replacement: %w", err)
	}

	return nil
}

// Sources returns manifest records in canonical logical-path order.
func (s *ManifestStore) Sources(ctx context.Context) ([]domain.Source, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT source_path, logical_path, size_bytes FROM sources ORDER BY logical_path COLLATE BINARY`)
	if err != nil {
		return nil, fmt.Errorf("query manifest sources: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var sources []domain.Source
	for rows.Next() {
		var source domain.Source
		if err := rows.Scan(&source.SourcePath, &source.LogicalPath, &source.Size); err != nil {
			return nil, fmt.Errorf("scan manifest source: %w", err)
		}
		sources = append(sources, source)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate manifest sources: %w", err)
	}

	return sources, nil
}

func (s *ManifestStore) initialize(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("create manifest metadata: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS sources (source_path TEXT NOT NULL, logical_path TEXT NOT NULL UNIQUE, size_bytes INTEGER NOT NULL)`); err != nil {
		return fmt.Errorf("create manifest sources: %w", err)
	}

	var version string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM metadata WHERE key = 'format_version'`).Scan(&version)
	if err == sql.ErrNoRows {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO metadata (key, value) VALUES ('format_version', ?)`, fmt.Sprint(manifestFormatVersion)); err != nil {
			return fmt.Errorf("write manifest format version: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("read manifest format version: %w", err)
	}
	if version != fmt.Sprint(manifestFormatVersion) {
		return fmt.Errorf("unsupported manifest format version %q", version)
	}

	return nil
}
