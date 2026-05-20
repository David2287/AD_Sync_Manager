package interfaces

import (
	"context"

	"ad-sync-manager/internal/domain/entity"
)

// ADClient is the port for all Active Directory operations.
// The production implementation uses LDAP over TLS (ldaps://); a stub
// implementation is used in unit tests.
type ADClient interface {
	// Authenticate binds with the supplied credentials and returns the user's
	// AD group DNs on success, or an error if authentication fails.
	Authenticate(ctx context.Context, username, password string) (groups []string, err error)

	// SyncEmployees pages through the configured EmployeeOU and returns every
	// record with the four synced attributes populated.
	SyncEmployees(ctx context.Context) ([]*entity.Employee, error)

	// GetEmployee retrieves a single employee by sAMAccountName.
	GetEmployee(ctx context.Context, username string) (*entity.Employee, error)

	// Close releases the underlying LDAP connection pool.
	Close() error
}
