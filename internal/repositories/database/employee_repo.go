package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, no cgo needed in Docker

	"ad-sync-manager/internal/domain/entity"
	"ad-sync-manager/internal/domain/interfaces"
)

type employeeRepo struct {
	db *sql.DB
}

// NewEmployeeRepo opens (or creates) a SQLite database and returns an
// interfaces.EmployeeRepository. The schema is applied automatically.
func NewEmployeeRepo(dsn string) (interfaces.EmployeeRepository, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("db: open: %w", err)
	}
	if err := applySchema(db); err != nil {
		return nil, fmt.Errorf("db: schema: %w", err)
	}
	return &employeeRepo{db: db}, nil
}

func applySchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS employees (
			id           TEXT PRIMARY KEY,
			display_name TEXT NOT NULL DEFAULT '',
			mail         TEXT NOT NULL DEFAULT '',
			phone        TEXT NOT NULL DEFAULT '',
			office       TEXT NOT NULL DEFAULT '',
			groups       TEXT NOT NULL DEFAULT '',   -- JSON array of DN strings
			synced_at    DATETIME NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_employees_office ON employees(office);
	`)
	return err
}

func (r *employeeRepo) Upsert(ctx context.Context, employees []*entity.Employee) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO employees (id, display_name, mail, phone, office, groups, synced_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			display_name = excluded.display_name,
			mail         = excluded.mail,
			phone        = excluded.phone,
			office       = excluded.office,
			groups       = excluded.groups,
			synced_at    = excluded.synced_at
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, e := range employees {
		groups := strings.Join(e.Groups, "\n") // newline-delimited; JSONification deferred to Stage 2
		if _, err := stmt.ExecContext(ctx, e.ID, e.DisplayName, e.Mail, e.Phone, e.Office, groups, e.SyncedAt); err != nil {
			return fmt.Errorf("db: upsert %q: %w", e.ID, err)
		}
	}

	return tx.Commit()
}

func (r *employeeRepo) FindByID(ctx context.Context, id string) (*entity.Employee, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, display_name, mail, phone, office, groups, synced_at
		   FROM employees WHERE id = ?`, id)
	return scanEmployee(row)
}

func (r *employeeRepo) List(ctx context.Context, f interfaces.EmployeeFilter) ([]*entity.Employee, error) {
	where := []string{"1=1"}
	args := []any{}

	if f.Office != "" {
		where = append(where, "office = ?")
		args = append(args, f.Office)
	}
	if f.Search != "" {
		where = append(where, "(display_name LIKE ? OR mail LIKE ?)")
		like := "%" + f.Search + "%"
		args = append(args, like, like)
	}

	if f.PageSize <= 0 {
		f.PageSize = 50
	}
	if f.Page <= 0 {
		f.Page = 1
	}

	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	query := fmt.Sprintf(
		`SELECT id, display_name, mail, phone, office, groups, synced_at
		   FROM employees WHERE %s
		   ORDER BY display_name
		   LIMIT ? OFFSET ?`,
		strings.Join(where, " AND "),
	)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*entity.Employee
	for rows.Next() {
		emp, err := scanEmployee(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, emp)
	}
	return out, rows.Err()
}

func (r *employeeRepo) Count(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM employees`).Scan(&n)
	return n, err
}

// scanner abstracts *sql.Row and *sql.Rows so one scan func covers both.
type scanner interface {
	Scan(dest ...any) error
}

func scanEmployee(s scanner) (*entity.Employee, error) {
	var (
		e        entity.Employee
		groups   string
		syncedAt time.Time
	)
	if err := s.Scan(&e.ID, &e.DisplayName, &e.Mail, &e.Phone, &e.Office, &groups, &syncedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("employee not found")
		}
		return nil, err
	}
	e.SyncedAt = syncedAt
	if groups != "" {
		e.Groups = strings.Split(groups, "\n")
	}
	return &e, nil
}
