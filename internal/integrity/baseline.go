package integrity

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Mismatch describes a detected integrity violation for a single employee DN.
type Mismatch struct {
	DN        string    `json:"dn"`
	OldHash   string    `json:"old_hash"`
	NewHash   string    `json:"new_hash"`
	CheckedAt time.Time `json:"checked_at"`
}

type baselineRow struct {
	DN        string
	Hash      string
	UpdatedAt time.Time
}

// BaselineStore manages the integrity_baseline table in a relational database.
type BaselineStore struct {
	db *sql.DB
}

// NewBaselineStore opens (or creates) a database connection and ensures the
// integrity_baseline table exists. driver is "sqlite" or "postgres"; dsn is
// the data-source name (file path for SQLite, connection URL for PostgreSQL).
func NewBaselineStore(driver, dsn string) (*BaselineStore, error) {
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("integrity: open db: %w", err)
	}
	if err := applyBaselineSchema(db); err != nil {
		db.Close() //nolint:errcheck
		return nil, fmt.Errorf("integrity: schema: %w", err)
	}
	return &BaselineStore{db: db}, nil
}

func applyBaselineSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS integrity_baseline (
			dn         TEXT PRIMARY KEY,
			hash       TEXT     NOT NULL DEFAULT '',
			updated_at DATETIME NOT NULL
		)
	`)
	return err
}

// GetBaseline returns the stored baseline for dn, or (nil, nil) when not found.
func (s *BaselineStore) GetBaseline(ctx context.Context, dn string) (*baselineRow, error) {
	row := s.db.QueryRowContext(ctx,
		"SELECT dn, hash, updated_at FROM integrity_baseline WHERE dn = ?", dn)
	var e baselineRow
	if err := row.Scan(&e.DN, &e.Hash, &e.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("integrity: get baseline: %w", err)
	}
	return &e, nil
}

// SaveOrUpdateBaseline upserts a baseline record for dn with the given hash.
func (s *BaselineStore) SaveOrUpdateBaseline(ctx context.Context, dn, hash string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO integrity_baseline (dn, hash, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(dn) DO UPDATE SET
			hash       = excluded.hash,
			updated_at = excluded.updated_at
	`, dn, hash, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("integrity: upsert baseline: %w", err)
	}
	return nil
}

// GetAllBaselines returns all stored baseline entries.
func (s *BaselineStore) GetAllBaselines(ctx context.Context) ([]baselineRow, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT dn, hash, updated_at FROM integrity_baseline")
	if err != nil {
		return nil, fmt.Errorf("integrity: list baselines: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	var entries []baselineRow
	for rows.Next() {
		var e baselineRow
		if err := rows.Scan(&e.DN, &e.Hash, &e.UpdatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// Close releases the database connection.
func (s *BaselineStore) Close() error { return s.db.Close() }
