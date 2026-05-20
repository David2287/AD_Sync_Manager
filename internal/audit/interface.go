package audit

import (
	"context"
	"time"
)

// AuditLog is a single audit trail entry stored in both the file and database backends.
type AuditLog struct {
	ID        int       `json:"id" db:"id"`
	Timestamp time.Time `json:"timestamp" db:"timestamp"`
	Operator  string    `json:"operator" db:"operator"`   // sAMAccountName from JWT
	Action    string    `json:"action" db:"action"`       // see Action* constants
	TargetDN  string    `json:"targetDN,omitempty" db:"target_dn"`
	Attribute string    `json:"attribute,omitempty" db:"attribute"`
	OldValue  string    `json:"oldValue,omitempty" db:"old_value"`
	NewValue  string    `json:"newValue,omitempty" db:"new_value"`
	Status    string    `json:"status" db:"status"`       // see Status* constants
	Details   string    `json:"details,omitempty" db:"details"` // JSON blob for extra info
	IPAddress string    `json:"ipAddress,omitempty" db:"ip_address"`
}

// Action constants identify the type of audited event.
const (
	ActionLogin          = "login"
	ActionApplyMarkdown  = "apply_markdown"
	ActionUpdateEmployee = "update_employee"
	ActionIntegrityCheck = "integrity_check" // Stage 6 placeholder
)

// Status constants indicate the outcome of the audited event.
const (
	StatusSuccess = "success"
	StatusFailure = "failure"
)

// AuditLogger accepts and persists audit events.
type AuditLogger interface {
	Log(entry AuditLog) error
	Close() error
}

// AuditQuerier reads audit entries from the database backend.
// Only DBLogger implements this interface; FileLogger does not support querying.
type AuditQuerier interface {
	List(ctx context.Context, filter LogFilter) ([]AuditLog, int, error)
	GetByID(ctx context.Context, id int) (*AuditLog, error)
}

// LogFilter defines optional filters for list queries.
type LogFilter struct {
	From     *time.Time
	To       *time.Time
	Operator string
	Action   string
	Status   string
	Limit    int
	Offset   int
}

// OperationChange describes one AD attribute modification in an apply_markdown event.
// Using a local type avoids importing the markdown package from this package.
type OperationChange struct {
	DN        string `json:"dn"`
	Attribute string `json:"attribute"`
	OldValue  string `json:"oldValue"`
	NewValue  string `json:"newValue"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
}
