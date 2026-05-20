package audit

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // SQLite driver (no cgo); already a project dependency.
	// To enable PostgreSQL: add _ "github.com/lib/pq" and change driver to "postgres".
	// Replace ? placeholders with $1, $2 … for the PostgreSQL dialect.
)

// DBLogger writes audit entries to a relational database and implements
// AuditQuerier so the /logs API can query them.
//
// Supported drivers:
//   - "sqlite"   — modernc.org/sqlite (default; embedded, no extra setup)
//   - "postgres" — add github.com/lib/pq to go.mod and blank-import it
//
// The schema is applied automatically on construction; no separate migration
// step is needed for SQLite. PostgreSQL users should run the SQL file in
// migrations/001_create_audit_logs.sql before the first deployment.
type DBLogger struct {
	db *sql.DB
}

// NewDBLogger opens (or creates) the audit database, applies the schema, and
// returns a DBLogger. driver is "sqlite" or "postgres"; dsn is the data-source
// name (e.g. a file path for SQLite or a connection URL for PostgreSQL).
func NewDBLogger(driver, dsn string) (*DBLogger, error) {
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("audit db: open: %w", err)
	}
	if err := applyAuditSchema(db); err != nil {
		db.Close() //nolint:errcheck
		return nil, fmt.Errorf("audit db: schema: %w", err)
	}
	return &DBLogger{db: db}, nil
}

func applyAuditSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS audit_logs (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp  DATETIME    NOT NULL,
			operator   TEXT        NOT NULL DEFAULT '',
			action     TEXT        NOT NULL DEFAULT '',
			target_dn  TEXT        NOT NULL DEFAULT '',
			attribute  TEXT        NOT NULL DEFAULT '',
			old_value  TEXT        NOT NULL DEFAULT '',
			new_value  TEXT        NOT NULL DEFAULT '',
			status     TEXT        NOT NULL DEFAULT '',
			details    TEXT        NOT NULL DEFAULT '',
			ip_address TEXT        NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_logs (timestamp);
		CREATE INDEX IF NOT EXISTS idx_audit_operator  ON audit_logs (operator);
		CREATE INDEX IF NOT EXISTS idx_audit_action    ON audit_logs (action);
	`)
	return err
}

// Log inserts one audit entry into the database.
func (d *DBLogger) Log(entry AuditLog) error {
	_, err := d.db.Exec(`
		INSERT INTO audit_logs
			(timestamp, operator, action, target_dn, attribute,
			 old_value, new_value, status, details, ip_address)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.Timestamp, entry.Operator, entry.Action,
		entry.TargetDN, entry.Attribute,
		entry.OldValue, entry.NewValue,
		entry.Status, entry.Details, entry.IPAddress,
	)
	if err != nil {
		return fmt.Errorf("audit db: insert: %w", err)
	}
	return nil
}

// List returns a filtered, paginated slice of audit entries and the total count
// of matching rows (ignoring pagination), suitable for the /logs API response.
func (d *DBLogger) List(ctx context.Context, f LogFilter) ([]AuditLog, int, error) {
	where, args := buildWhereClause(f)

	var total int
	if err := d.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM audit_logs"+where, args...,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("audit db: count: %w", err)
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	rows, err := d.db.QueryContext(ctx,
		"SELECT id, timestamp, operator, action, target_dn, attribute,"+
			" old_value, new_value, status, details, ip_address"+
			" FROM audit_logs"+where+
			" ORDER BY timestamp DESC LIMIT ? OFFSET ?",
		append(args, limit, offset)...,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("audit db: list: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var entries []AuditLog
	for rows.Next() {
		var e AuditLog
		if err := scanLog(rows, &e); err != nil {
			return nil, 0, err
		}
		entries = append(entries, e)
	}
	return entries, total, rows.Err()
}

// GetByID retrieves a single audit entry by its primary key.
// Returns (nil, nil) when the entry does not exist.
func (d *DBLogger) GetByID(ctx context.Context, id int) (*AuditLog, error) {
	row := d.db.QueryRowContext(ctx,
		"SELECT id, timestamp, operator, action, target_dn, attribute,"+
			" old_value, new_value, status, details, ip_address"+
			" FROM audit_logs WHERE id = ?", id)
	var e AuditLog
	if err := scanLog(row, &e); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("audit db: get by id: %w", err)
	}
	return &e, nil
}

// Close releases the database connection pool.
func (d *DBLogger) Close() error { return d.db.Close() }

// ── internal helpers ──────────────────────────────────────────────────────────

type rowScanner interface {
	Scan(dest ...any) error
}

func scanLog(s rowScanner, e *AuditLog) error {
	return s.Scan(
		&e.ID, &e.Timestamp, &e.Operator, &e.Action,
		&e.TargetDN, &e.Attribute,
		&e.OldValue, &e.NewValue,
		&e.Status, &e.Details, &e.IPAddress,
	)
}

func buildWhereClause(f LogFilter) (string, []any) {
	var conds []string
	var args []any

	if f.From != nil {
		conds = append(conds, "timestamp >= ?")
		args = append(args, f.From.UTC().Format(time.RFC3339Nano))
	}
	if f.To != nil {
		conds = append(conds, "timestamp <= ?")
		args = append(args, f.To.UTC().Format(time.RFC3339Nano))
	}
	if f.Operator != "" {
		conds = append(conds, "operator = ?")
		args = append(args, f.Operator)
	}
	if f.Action != "" {
		conds = append(conds, "action = ?")
		args = append(args, f.Action)
	}
	if f.Status != "" {
		conds = append(conds, "status = ?")
		args = append(args, f.Status)
	}

	if len(conds) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}
