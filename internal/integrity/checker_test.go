package integrity

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ad-sync-manager/internal/audit"
)

// ── test helpers ──────────────────────────────────────────────────────────────

func newTestStore(t *testing.T) *BaselineStore {
	t.Helper()
	store, err := NewBaselineStore("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() }) //nolint:errcheck
	return store
}

func staticLister(emps []EmployeeSnapshot) EmployeeLister {
	return func() ([]EmployeeSnapshot, error) { return emps, nil }
}

// capLogger captures audit entries for assertion.
type capLogger struct {
	entries []audit.AuditLog
}

func (l *capLogger) Log(e audit.AuditLog) error {
	l.entries = append(l.entries, e)
	return nil
}

func (l *capLogger) Close() error { return nil }

// ── computeHash ───────────────────────────────────────────────────────────────

func TestComputeHash_Stable(t *testing.T) {
	e := EmployeeSnapshot{DN: "CN=A", DisplayName: "Alice", Mail: "a@x.com", Phone: "+1", Office: "B101"}
	assert.Equal(t, computeHash(e), computeHash(e), "hash must be deterministic")
	assert.Len(t, computeHash(e), 64, "SHA-256 hex string is 64 chars")
}

func TestComputeHash_PhoneChange(t *testing.T) {
	a := EmployeeSnapshot{DN: "CN=A", Phone: "111"}
	b := EmployeeSnapshot{DN: "CN=A", Phone: "222"}
	assert.NotEqual(t, computeHash(a), computeHash(b))
}

func TestComputeHash_DNChange(t *testing.T) {
	a := EmployeeSnapshot{DN: "CN=A", DisplayName: "Alice"}
	b := EmployeeSnapshot{DN: "CN=B", DisplayName: "Alice"}
	assert.NotEqual(t, computeHash(a), computeHash(b))
}

func TestComputeHash_AllFieldsDistinct(t *testing.T) {
	base := EmployeeSnapshot{DN: "CN=X", DisplayName: "X", Mail: "x@x.com", Phone: "1", Office: "A"}
	variants := []EmployeeSnapshot{
		{DN: "CN=Y", DisplayName: "X", Mail: "x@x.com", Phone: "1", Office: "A"},
		{DN: "CN=X", DisplayName: "Z", Mail: "x@x.com", Phone: "1", Office: "A"},
		{DN: "CN=X", DisplayName: "X", Mail: "z@x.com", Phone: "1", Office: "A"},
		{DN: "CN=X", DisplayName: "X", Mail: "x@x.com", Phone: "9", Office: "A"},
		{DN: "CN=X", DisplayName: "X", Mail: "x@x.com", Phone: "1", Office: "Z"},
	}
	baseHash := computeHash(base)
	for _, v := range variants {
		assert.NotEqual(t, baseHash, computeHash(v), "each field must affect the hash")
	}
}

// ── RunNow ────────────────────────────────────────────────────────────────────

func TestRunNow_NoPriorBaseline(t *testing.T) {
	store := newTestStore(t)
	c := NewChecker(store, staticLister([]EmployeeSnapshot{
		{DN: "CN=Alice,DC=x,DC=com", DisplayName: "Alice"},
	}), nil, false)

	mismatches, total, err := c.RunNow(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Empty(t, mismatches, "first run establishes baseline — no mismatches expected")
}

func TestRunNow_DetectsMismatch(t *testing.T) {
	store := newTestStore(t)

	// Establish baseline.
	c1 := NewChecker(store, staticLister([]EmployeeSnapshot{
		{DN: "CN=A,DC=x,DC=com", Phone: "+1 111 111"},
	}), nil, false)
	_, _, err := c1.RunNow(context.Background())
	require.NoError(t, err)

	// Phone changed outside the application.
	c2 := NewChecker(store, staticLister([]EmployeeSnapshot{
		{DN: "CN=A,DC=x,DC=com", Phone: "+9 999 999"},
	}), nil, false)
	mismatches, total, err := c2.RunNow(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, mismatches, 1)
	assert.Equal(t, "CN=A,DC=x,DC=com", mismatches[0].DN)
	assert.NotEqual(t, mismatches[0].OldHash, mismatches[0].NewHash)
	assert.False(t, mismatches[0].CheckedAt.IsZero())
}

func TestRunNow_NoMismatch_WhenDataUnchanged(t *testing.T) {
	store := newTestStore(t)
	emps := []EmployeeSnapshot{{DN: "CN=A,DC=x,DC=com", Phone: "+1 555"}}
	c := NewChecker(store, staticLister(emps), nil, false)

	_, _, _ = c.RunNow(context.Background()) // establish baseline

	mismatches, _, err := c.RunNow(context.Background())
	require.NoError(t, err)
	assert.Empty(t, mismatches)
}

func TestRunNow_MultipleEmployees_PartialMismatch(t *testing.T) {
	store := newTestStore(t)

	// Establish baseline for two employees.
	c1 := NewChecker(store, staticLister([]EmployeeSnapshot{
		{DN: "CN=Alice,DC=x,DC=com", Phone: "111"},
		{DN: "CN=Bob,DC=x,DC=com", Phone: "222"},
	}), nil, false)
	_, _, _ = c1.RunNow(context.Background())

	// Only Bob changes.
	c2 := NewChecker(store, staticLister([]EmployeeSnapshot{
		{DN: "CN=Alice,DC=x,DC=com", Phone: "111"},
		{DN: "CN=Bob,DC=x,DC=com", Phone: "999"},
	}), nil, false)
	mismatches, total, err := c2.RunNow(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	require.Len(t, mismatches, 1)
	assert.Equal(t, "CN=Bob,DC=x,DC=com", mismatches[0].DN)
}

// ── AutoUpdate ────────────────────────────────────────────────────────────────

func TestRunNow_AutoUpdate_NewBaselineAfterMismatch(t *testing.T) {
	store := newTestStore(t)

	// Establish baseline.
	c1 := NewChecker(store, staticLister([]EmployeeSnapshot{{DN: "CN=A", Phone: "old"}}), nil, false)
	_, _, _ = c1.RunNow(context.Background())

	// Mismatch detected with autoUpdate=true — baseline overwritten.
	c2 := NewChecker(store, staticLister([]EmployeeSnapshot{{DN: "CN=A", Phone: "new"}}), nil, true)
	mismatches, _, _ := c2.RunNow(context.Background())
	assert.Len(t, mismatches, 1)

	// Third run with the same "new" phone — no mismatch because baseline was updated.
	c3 := NewChecker(store, staticLister([]EmployeeSnapshot{{DN: "CN=A", Phone: "new"}}), nil, false)
	mismatches, _, _ = c3.RunNow(context.Background())
	assert.Empty(t, mismatches)
}

func TestRunNow_NoAutoUpdate_BaselineUnchangedAfterMismatch(t *testing.T) {
	store := newTestStore(t)

	c1 := NewChecker(store, staticLister([]EmployeeSnapshot{{DN: "CN=A", Phone: "old"}}), nil, false)
	_, _, _ = c1.RunNow(context.Background())

	// Mismatch with autoUpdate=false — baseline not updated.
	c2 := NewChecker(store, staticLister([]EmployeeSnapshot{{DN: "CN=A", Phone: "new"}}), nil, false)
	_, _, _ = c2.RunNow(context.Background())

	// Second run still sees the mismatch.
	c3 := NewChecker(store, staticLister([]EmployeeSnapshot{{DN: "CN=A", Phone: "new"}}), nil, false)
	mismatches, _, _ := c3.RunNow(context.Background())
	assert.Len(t, mismatches, 1, "baseline unchanged — mismatch must persist")
}

// ── Audit logging ─────────────────────────────────────────────────────────────

func TestRunNow_LogsViolationEntryPerMismatch(t *testing.T) {
	store := newTestStore(t)
	ml := &capLogger{}

	c1 := NewChecker(store, staticLister([]EmployeeSnapshot{{DN: "CN=A", Phone: "old"}}), nil, false)
	_, _, _ = c1.RunNow(context.Background())

	c2 := NewChecker(store, staticLister([]EmployeeSnapshot{{DN: "CN=A", Phone: "new"}}), ml, false)
	_, _, _ = c2.RunNow(context.Background())

	var found bool
	for _, e := range ml.entries {
		if e.Action == audit.ActionIntegrityCheck && e.Status == audit.StatusFailure && e.TargetDN == "CN=A" {
			found = true
		}
	}
	assert.True(t, found, "expected integrity_check failure entry for CN=A")
}

func TestRunNow_LogsSummaryEntry(t *testing.T) {
	store := newTestStore(t)
	ml := &capLogger{}

	c := NewChecker(store, staticLister([]EmployeeSnapshot{{DN: "CN=A", Phone: "123"}}), ml, false)
	_, _, _ = c.RunNow(context.Background())

	var hasSummary bool
	for _, e := range ml.entries {
		if e.Action == audit.ActionIntegrityCheck && e.TargetDN == "" {
			hasSummary = true // summary entries have no TargetDN
		}
	}
	assert.True(t, hasSummary, "expected summary audit entry with no TargetDN")
}

func TestRunNow_SummaryStatus_SuccessWhenNoMismatches(t *testing.T) {
	store := newTestStore(t)
	ml := &capLogger{}
	emps := []EmployeeSnapshot{{DN: "CN=A", Phone: "123"}}

	c := NewChecker(store, staticLister(emps), nil, false)
	_, _, _ = c.RunNow(context.Background()) // establish baseline

	ml2 := &capLogger{}
	c2 := NewChecker(store, staticLister(emps), ml2, false)
	_, _, _ = c2.RunNow(context.Background())

	for _, e := range ml2.entries {
		if e.Action == audit.ActionIntegrityCheck && e.TargetDN == "" {
			assert.Equal(t, audit.StatusSuccess, e.Status)
		}
	}
}

// ── Reset ─────────────────────────────────────────────────────────────────────

func TestReset_ReportsAndOverwritesBaseline(t *testing.T) {
	store := newTestStore(t)

	// Establish old baseline.
	c1 := NewChecker(store, staticLister([]EmployeeSnapshot{{DN: "CN=A", Phone: "old"}}), nil, false)
	_, _, _ = c1.RunNow(context.Background())

	// Reset with new data — should report one mismatch and overwrite.
	c2 := NewChecker(store, staticLister([]EmployeeSnapshot{{DN: "CN=A", Phone: "brand-new"}}), nil, false)
	mismatches, total, err := c2.Reset(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, mismatches, 1)

	// After reset, same data must produce no mismatches.
	c3 := NewChecker(store, staticLister([]EmployeeSnapshot{{DN: "CN=A", Phone: "brand-new"}}), nil, false)
	mismatches, _, err = c3.RunNow(context.Background())
	require.NoError(t, err)
	assert.Empty(t, mismatches)
}

func TestReset_ClearsLastResult(t *testing.T) {
	store := newTestStore(t)

	// Establish baseline then detect a mismatch.
	c1 := NewChecker(store, staticLister([]EmployeeSnapshot{{DN: "CN=A", Phone: "old"}}), nil, false)
	_, _, _ = c1.RunNow(context.Background())
	c2 := NewChecker(store, staticLister([]EmployeeSnapshot{{DN: "CN=A", Phone: "new"}}), nil, false)
	_, _, _ = c2.RunNow(context.Background())
	assert.Len(t, c2.LastResult(), 1)

	// After Reset, LastResult must be empty.
	c3 := NewChecker(store, staticLister([]EmployeeSnapshot{{DN: "CN=A", Phone: "new"}}), nil, false)
	_, _, _ = c3.Reset(context.Background())
	assert.Empty(t, c3.LastResult())
}

// ── LastResult ────────────────────────────────────────────────────────────────

func TestLastResult_NeverNil(t *testing.T) {
	store := newTestStore(t)
	c := NewChecker(store, staticLister(nil), nil, false)
	result := c.LastResult()
	assert.NotNil(t, result, "LastResult must never return nil")
}

func TestLastResult_ReflectsLatestRun(t *testing.T) {
	store := newTestStore(t)

	c1 := NewChecker(store, staticLister([]EmployeeSnapshot{{DN: "CN=A", Phone: "old"}}), nil, false)
	_, _, _ = c1.RunNow(context.Background())

	c2 := NewChecker(store, staticLister([]EmployeeSnapshot{{DN: "CN=A", Phone: "new"}}), nil, false)
	_, _, _ = c2.RunNow(context.Background())
	assert.Len(t, c2.LastResult(), 1)

	// Run again with unchanged data — mismatch list should now be empty.
	c3 := NewChecker(store, staticLister([]EmployeeSnapshot{{DN: "CN=A", Phone: "new"}}), nil, true)
	_, _, _ = c3.RunNow(context.Background()) // autoUpdate repairs baseline
	c4 := NewChecker(store, staticLister([]EmployeeSnapshot{{DN: "CN=A", Phone: "new"}}), nil, false)
	_, _, _ = c4.RunNow(context.Background())
	assert.Empty(t, c4.LastResult())
}
