package interfaces

import (
	"context"
	"time"
)

// SyncResult summarises a completed sync run.
type SyncResult struct {
	StartedAt  time.Time
	FinishedAt time.Time
	Upserted   int
	Errors     []string
}

// SyncService is the port for the AD-sync use-case.
type SyncService interface {
	// Run triggers an immediate full sync from AD → local DB.
	Run(ctx context.Context) (*SyncResult, error)

	// LastResult returns the outcome of the most recent sync run.
	LastResult(ctx context.Context) (*SyncResult, error)
}
