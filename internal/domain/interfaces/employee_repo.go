package interfaces

import (
	"context"

	"ad-sync-manager/internal/domain/entity"
)

// EmployeeFilter controls list queries.
type EmployeeFilter struct {
	Office   string // exact match
	Search   string // substring across DisplayName and Mail
	Page     int    // 1-based
	PageSize int    // max rows (default 50)
}

// EmployeeRepository is the port for local employee persistence (SQLite).
type EmployeeRepository interface {
	// Upsert inserts or updates a batch of employees atomically.
	Upsert(ctx context.Context, employees []*entity.Employee) error

	// FindByID looks up an employee by sAMAccountName.
	FindByID(ctx context.Context, id string) (*entity.Employee, error)

	// List returns a filtered, paginated slice of employees.
	List(ctx context.Context, filter EmployeeFilter) ([]*entity.Employee, error)

	// Count returns total number of synced employees.
	Count(ctx context.Context) (int64, error)
}
