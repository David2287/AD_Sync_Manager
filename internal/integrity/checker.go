package integrity

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"ad-sync-manager/internal/audit"
)

// EmployeeSnapshot is the subset of employee data hashed for integrity checking.
// Using a local type keeps the integrity package free of any dependency on the
// employee package, which would create a compile-time coupling that makes
// unit-testing with mocks harder.
type EmployeeSnapshot struct {
	DN          string
	DisplayName string
	Mail        string
	Phone       string
	Office      string
}

// EmployeeLister fetches all employee snapshots from the directory.
// In production, wire this to employee.GetAllEmployees; in tests, use a stub.
type EmployeeLister func() ([]EmployeeSnapshot, error)

// Checker compares current AD state against a stored baseline and emits audit
// log entries for every detected deviation.
type Checker struct {
	store      *BaselineStore
	lister     EmployeeLister
	logger     audit.AuditLogger // may be nil (violations are still returned)
	autoUpdate bool              // when true, mismatched baselines are re-hashed

	// running prevents concurrent integrity check runs (TryLock pattern).
	running sync.Mutex

	resultMu sync.RWMutex
	last     []Mismatch // mismatches from the most recent completed check
}

// NewChecker creates a Checker. Pass autoUpdate=true to overwrite the baseline
// immediately on detection; false logs only and leaves the baseline unchanged.
func NewChecker(store *BaselineStore, lister EmployeeLister, logger audit.AuditLogger, autoUpdate bool) *Checker {
	return &Checker{
		store:      store,
		lister:     lister,
		logger:     logger,
		autoUpdate: autoUpdate,
	}
}

// Start runs integrity checks on interval until ctx is cancelled.
// An initial check executes synchronously before the first ticker tick.
func (c *Checker) Start(ctx context.Context, interval time.Duration) {
	c.runOnce(ctx)

	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			c.runOnce(ctx)
		case <-ctx.Done():
			return
		}
	}
}

// RunNow triggers an immediate check and returns (mismatches, totalChecked, error).
// Returns an error when another check is already running.
func (c *Checker) RunNow(ctx context.Context) ([]Mismatch, int, error) {
	if !c.running.TryLock() {
		return nil, 0, fmt.Errorf("integrity: check already in progress")
	}
	defer c.running.Unlock()

	mismatches, total, err := c.doRun(ctx)
	if err == nil {
		c.resultMu.Lock()
		c.last = mismatches
		c.resultMu.Unlock()
	}
	return mismatches, total, err
}

// Reset fetches current AD state, records mismatches against the stored baseline,
// then overwrites every baseline entry with the fresh hash. Useful after a bulk
// legitimate update when the operator wants to acknowledge the changes.
func (c *Checker) Reset(ctx context.Context) ([]Mismatch, int, error) {
	if !c.running.TryLock() {
		return nil, 0, fmt.Errorf("integrity: check already in progress")
	}
	defer c.running.Unlock()

	employees, err := c.lister()
	if err != nil {
		return nil, 0, fmt.Errorf("integrity: list employees: %w", err)
	}

	var mismatches []Mismatch
	now := time.Now().UTC()
	for _, emp := range employees {
		current := computeHash(emp)
		baseline, _ := c.store.GetBaseline(ctx, emp.DN)
		if baseline != nil && baseline.Hash != current {
			mismatches = append(mismatches, Mismatch{
				DN:        emp.DN,
				OldHash:   baseline.Hash,
				NewHash:   current,
				CheckedAt: now,
			})
		}
		_ = c.store.SaveOrUpdateBaseline(ctx, emp.DN, current)
	}

	// After a manual reset the last-result slate is cleared.
	c.resultMu.Lock()
	c.last = []Mismatch{}
	c.resultMu.Unlock()

	return mismatches, len(employees), nil
}

// LastResult returns the mismatches from the most recently completed check.
// Returns an empty (never nil) slice before the first check completes.
func (c *Checker) LastResult() []Mismatch {
	c.resultMu.RLock()
	defer c.resultMu.RUnlock()
	if c.last == nil {
		return []Mismatch{}
	}
	return c.last
}

// ── internal ──────────────────────────────────────────────────────────────────

func (c *Checker) runOnce(ctx context.Context) {
	if !c.running.TryLock() {
		return // another run is already in progress — skip this tick
	}
	defer c.running.Unlock()

	mismatches, _, err := c.doRun(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integrity: check error: %v\n", err)
		return
	}

	c.resultMu.Lock()
	c.last = mismatches
	c.resultMu.Unlock()
}

// doRun performs one check cycle. The caller must hold c.running.
func (c *Checker) doRun(ctx context.Context) ([]Mismatch, int, error) {
	employees, err := c.lister()
	if err != nil {
		return nil, 0, fmt.Errorf("integrity: list employees: %w", err)
	}

	var mismatches []Mismatch
	now := time.Now().UTC()

	for _, emp := range employees {
		current := computeHash(emp)

		baseline, err := c.store.GetBaseline(ctx, emp.DN)
		if err != nil {
			fmt.Fprintf(os.Stderr, "integrity: get baseline %q: %v\n", emp.DN, err)
			continue
		}

		if baseline == nil {
			// First time seeing this DN — establish the baseline without flagging.
			_ = c.store.SaveOrUpdateBaseline(ctx, emp.DN, current)
			continue
		}

		if baseline.Hash != current {
			m := Mismatch{
				DN:        emp.DN,
				OldHash:   baseline.Hash,
				NewHash:   current,
				CheckedAt: now,
			}
			mismatches = append(mismatches, m)
			c.emitViolation(m)

			if c.autoUpdate {
				_ = c.store.SaveOrUpdateBaseline(ctx, emp.DN, current)
			}
		}
	}

	c.emitSummary(len(employees), len(mismatches), now)
	return mismatches, len(employees), nil
}

func (c *Checker) emitViolation(m Mismatch) {
	if c.logger == nil {
		return
	}
	details, _ := json.Marshal(m)
	_ = c.logger.Log(audit.AuditLog{
		Timestamp: m.CheckedAt,
		Operator:  "system",
		Action:    audit.ActionIntegrityCheck,
		TargetDN:  m.DN,
		Status:    audit.StatusFailure,
		Details:   string(details),
	})
}

func (c *Checker) emitSummary(total, mismatched int, t time.Time) {
	if c.logger == nil {
		return
	}
	type summaryPayload struct {
		TotalChecked int `json:"total_checked"`
		Mismatches   int `json:"mismatches"`
	}
	b, _ := json.Marshal(summaryPayload{total, mismatched})
	status := audit.StatusSuccess
	if mismatched > 0 {
		status = audit.StatusFailure
	}
	_ = c.logger.Log(audit.AuditLog{
		Timestamp: t,
		Operator:  "system",
		Action:    audit.ActionIntegrityCheck,
		Status:    status,
		Details:   string(b),
	})
}

// computeHash returns a SHA-256 hex digest of the employee's monitored attributes.
// The encoding format is stable — changing it invalidates all stored baselines.
func computeHash(e EmployeeSnapshot) string {
	h := sha256.New()
	fmt.Fprintf(h, "dn=%s|displayName=%s|mail=%s|phone=%s|office=%s",
		e.DN, e.DisplayName, e.Mail, e.Phone, e.Office)
	return fmt.Sprintf("%x", h.Sum(nil))
}
