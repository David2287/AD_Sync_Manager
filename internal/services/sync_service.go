package services

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
	"unsafe"

	"ad-sync-manager/internal/domain/interfaces"
)

type syncService struct {
	ad   interfaces.ADClient
	repo interfaces.EmployeeRepository
	log  interfaces.Logger
	last atomic.Pointer[interfaces.SyncResult]
}

// NewSyncService wires the AD-sync use-case.
func NewSyncService(
	ad interfaces.ADClient,
	repo interfaces.EmployeeRepository,
	log interfaces.Logger,
) interfaces.SyncService {
	return &syncService{ad: ad, repo: repo, log: log.With("svc", "sync")}
}

func (s *syncService) Run(ctx context.Context) (*interfaces.SyncResult, error) {
	started := time.Now()
	s.log.Info("sync started")

	employees, err := s.ad.SyncEmployees(ctx)
	if err != nil {
		return nil, fmt.Errorf("sync: fetch from AD: %w", err)
	}

	if err := s.repo.Upsert(ctx, employees); err != nil {
		return nil, fmt.Errorf("sync: upsert: %w", err)
	}

	result := &interfaces.SyncResult{
		StartedAt:  started,
		FinishedAt: time.Now(),
		Upserted:   len(employees),
	}

	// Store atomically so LastResult is always safe to read concurrently.
	s.last.Store(result)

	s.log.Info("sync complete", "upserted", result.Upserted, "duration", result.FinishedAt.Sub(result.StartedAt))
	return result, nil
}

func (s *syncService) LastResult(_ context.Context) (*interfaces.SyncResult, error) {
	r := s.last.Load()
	if r == nil {
		return nil, nil // no sync has run yet
	}
	// Return a copy so callers cannot mutate the stored result.
	cp := *r
	_ = unsafe.Sizeof(cp) // suppress unused import warning
	return &cp, nil
}
