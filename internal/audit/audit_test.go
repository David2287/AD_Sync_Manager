package audit_test

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ad-sync-manager/internal/audit"
)

// ── capture logger ────────────────────────────────────────────────────────────

// captureLogger implements AuditLogger and records every emitted entry.
type captureLogger struct {
	entries []audit.AuditLog
}

func (c *captureLogger) Log(e audit.AuditLog) error {
	c.entries = append(c.entries, e)
	return nil
}

func (c *captureLogger) Close() error { return nil }

// ── DBLogger ──────────────────────────────────────────────────────────────────

func newMemDB(t *testing.T) *audit.DBLogger {
	t.Helper()
	db, err := audit.NewDBLogger("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() }) //nolint:errcheck
	return db
}

func TestDBLogger_LogAndList(t *testing.T) {
	db := newMemDB(t)

	entry := audit.AuditLog{
		Timestamp: time.Now().UTC(),
		Operator:  "jdoe",
		Action:    audit.ActionLogin,
		Status:    audit.StatusSuccess,
		IPAddress: "10.0.0.1",
	}
	require.NoError(t, db.Log(entry))

	entries, total, err := db.List(context.Background(), audit.LogFilter{Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, entries, 1)
	assert.Equal(t, "jdoe", entries[0].Operator)
	assert.Equal(t, audit.ActionLogin, entries[0].Action)
	assert.Equal(t, audit.StatusSuccess, entries[0].Status)
	assert.Equal(t, "10.0.0.1", entries[0].IPAddress)
	assert.Greater(t, entries[0].ID, 0, "inserted row must get an auto-incremented ID")
}

func TestDBLogger_FilterByOperator(t *testing.T) {
	db := newMemDB(t)

	db.Log(audit.AuditLog{Timestamp: time.Now(), Operator: "alice", Action: audit.ActionLogin, Status: audit.StatusSuccess})   //nolint:errcheck
	db.Log(audit.AuditLog{Timestamp: time.Now(), Operator: "bob", Action: audit.ActionLogin, Status: audit.StatusSuccess})    //nolint:errcheck
	db.Log(audit.AuditLog{Timestamp: time.Now(), Operator: "alice", Action: audit.ActionUpdateEmployee, Status: audit.StatusSuccess}) //nolint:errcheck

	_, total, err := db.List(context.Background(), audit.LogFilter{Operator: "alice"})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
}

func TestDBLogger_FilterByAction(t *testing.T) {
	db := newMemDB(t)

	db.Log(audit.AuditLog{Timestamp: time.Now(), Action: audit.ActionLogin, Status: audit.StatusSuccess})          //nolint:errcheck
	db.Log(audit.AuditLog{Timestamp: time.Now(), Action: audit.ActionApplyMarkdown, Status: audit.StatusSuccess}) //nolint:errcheck

	_, total, err := db.List(context.Background(), audit.LogFilter{Action: audit.ActionLogin})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
}

func TestDBLogger_FilterByStatus(t *testing.T) {
	db := newMemDB(t)

	db.Log(audit.AuditLog{Timestamp: time.Now(), Action: audit.ActionLogin, Status: audit.StatusSuccess}) //nolint:errcheck
	db.Log(audit.AuditLog{Timestamp: time.Now(), Action: audit.ActionLogin, Status: audit.StatusFailure}) //nolint:errcheck

	_, total, err := db.List(context.Background(), audit.LogFilter{Status: audit.StatusFailure})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
}

func TestDBLogger_FilterByTimeRange(t *testing.T) {
	db := newMemDB(t)

	past := time.Now().Add(-2 * time.Hour).UTC()
	future := time.Now().Add(2 * time.Hour).UTC()
	now := time.Now().UTC()

	db.Log(audit.AuditLog{Timestamp: now, Action: audit.ActionLogin, Status: audit.StatusSuccess}) //nolint:errcheck

	_, total, err := db.List(context.Background(), audit.LogFilter{From: &past, To: &future})
	require.NoError(t, err)
	assert.Equal(t, 1, total)

	veryOld := now.Add(-5 * time.Hour)
	_, total, err = db.List(context.Background(), audit.LogFilter{To: &veryOld})
	require.NoError(t, err)
	assert.Equal(t, 0, total)
}

func TestDBLogger_Pagination(t *testing.T) {
	db := newMemDB(t)

	for i := 0; i < 5; i++ {
		db.Log(audit.AuditLog{Timestamp: time.Now(), Action: audit.ActionLogin, Status: audit.StatusSuccess}) //nolint:errcheck
	}

	entries, total, err := db.List(context.Background(), audit.LogFilter{Limit: 2, Offset: 0})
	require.NoError(t, err)
	assert.Equal(t, 5, total)
	assert.Len(t, entries, 2)

	entries2, _, err := db.List(context.Background(), audit.LogFilter{Limit: 2, Offset: 2})
	require.NoError(t, err)
	assert.Len(t, entries2, 2)
	assert.NotEqual(t, entries[0].ID, entries2[0].ID)
}

func TestDBLogger_GetByID(t *testing.T) {
	db := newMemDB(t)

	db.Log(audit.AuditLog{Timestamp: time.Now(), Operator: "tester", Action: audit.ActionLogin, Status: audit.StatusSuccess}) //nolint:errcheck
	entries, _, _ := db.List(context.Background(), audit.LogFilter{Limit: 1})
	require.Len(t, entries, 1)

	got, err := db.GetByID(context.Background(), entries[0].ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "tester", got.Operator)
	assert.Equal(t, entries[0].ID, got.ID)
}

func TestDBLogger_GetByID_NotFound(t *testing.T) {
	db := newMemDB(t)

	got, err := db.GetByID(context.Background(), 99999)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestDBLogger_EmptyList_ReturnsZeroTotal(t *testing.T) {
	db := newMemDB(t)
	entries, total, err := db.List(context.Background(), audit.LogFilter{Limit: 50})
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, entries)
}

// ── MultiLogger ───────────────────────────────────────────────────────────────

func TestMultiLogger_FanOutToAll(t *testing.T) {
	a := &captureLogger{}
	b := &captureLogger{}
	m := audit.NewMultiLogger(a, b)

	entry := audit.AuditLog{Operator: "x", Action: audit.ActionLogin, Status: audit.StatusSuccess}
	require.NoError(t, m.Log(entry))
	require.NoError(t, m.Close())

	assert.Len(t, a.entries, 1, "logger A must receive the entry")
	assert.Len(t, b.entries, 1, "logger B must receive the entry")
	assert.Equal(t, "x", a.entries[0].Operator)
}

func TestMultiLogger_ContinuesAfterFirstError(t *testing.T) {
	bad := &errorLogger{}
	good := &captureLogger{}
	m := audit.NewMultiLogger(bad, good)

	_ = m.Log(audit.AuditLog{Action: audit.ActionLogin, Status: audit.StatusSuccess})

	// The good logger must still receive the entry even though the bad one errored.
	assert.Len(t, good.entries, 1)
}

// errorLogger always returns an error from Log.
type errorLogger struct{}

func (e *errorLogger) Log(_ audit.AuditLog) error { return assert.AnError }
func (e *errorLogger) Close() error               { return nil }

// ── AsyncLogger ───────────────────────────────────────────────────────────────

func TestAsyncLogger_DrainOnClose(t *testing.T) {
	inner := &captureLogger{}
	a := audit.NewAsyncLogger(inner, 64)

	for i := 0; i < 5; i++ {
		a.Log(audit.AuditLog{Timestamp: time.Now(), Action: audit.ActionLogin, Status: audit.StatusSuccess}) //nolint:errcheck
	}

	require.NoError(t, a.Close())
	assert.Len(t, inner.entries, 5, "all buffered entries must be drained before Close returns")
}

func TestAsyncLogger_NonBlocking_DropOnOverflow(t *testing.T) {
	inner := &captureLogger{}
	bufSize := 2
	a := audit.NewAsyncLogger(inner, bufSize)

	// Send more entries than the buffer can hold without consuming them first.
	for i := 0; i < bufSize+4; i++ {
		a.Log(audit.AuditLog{Timestamp: time.Now(), Action: audit.ActionLogin, Status: audit.StatusSuccess}) //nolint:errcheck
	}

	// Close drains whatever made it into the buffer.
	require.NoError(t, a.Close())
	assert.LessOrEqual(t, len(inner.entries), bufSize+4, "entries delivered ≤ total sent")
}

// ── Audit helper functions ────────────────────────────────────────────────────

func TestLogLogin_SuccessEntry(t *testing.T) {
	ml := &captureLogger{}
	audit.Init(ml)
	t.Cleanup(func() { audit.Init(nil) })

	audit.LogLogin("jdoe", "10.0.0.1", true, "")

	require.Len(t, ml.entries, 1)
	e := ml.entries[0]
	assert.Equal(t, audit.ActionLogin, e.Action)
	assert.Equal(t, audit.StatusSuccess, e.Status)
	assert.Equal(t, "jdoe", e.Operator)
	assert.Equal(t, "10.0.0.1", e.IPAddress)
	assert.Empty(t, e.Details)
	assert.False(t, e.Timestamp.IsZero())
}

func TestLogLogin_FailureEntry(t *testing.T) {
	ml := &captureLogger{}
	audit.Init(ml)
	t.Cleanup(func() { audit.Init(nil) })

	audit.LogLogin("bad_user", "10.0.0.2", false, "invalid credentials")

	require.Len(t, ml.entries, 1)
	e := ml.entries[0]
	assert.Equal(t, audit.StatusFailure, e.Status)
	assert.Contains(t, e.Details, "invalid credentials")
}

func TestLogUpdateEmployee_SuccessEntry(t *testing.T) {
	ml := &captureLogger{}
	audit.Init(ml)
	t.Cleanup(func() { audit.Init(nil) })

	audit.LogUpdateEmployee("admin", "10.0.0.1",
		"CN=A,DC=x", "telephoneNumber", "old-phone", "+1 555", true, "")

	require.Len(t, ml.entries, 1)
	e := ml.entries[0]
	assert.Equal(t, audit.ActionUpdateEmployee, e.Action)
	assert.Equal(t, audit.StatusSuccess, e.Status)
	assert.Equal(t, "CN=A,DC=x", e.TargetDN)
	assert.Equal(t, "telephoneNumber", e.Attribute)
	assert.Equal(t, "old-phone", e.OldValue)
	assert.Equal(t, "+1 555", e.NewValue)
	assert.Empty(t, e.Details)
}

func TestLogUpdateEmployee_FailureEntry(t *testing.T) {
	ml := &captureLogger{}
	audit.Init(ml)
	t.Cleanup(func() { audit.Init(nil) })

	audit.LogUpdateEmployee("admin", "10.0.0.1",
		"CN=A,DC=x", "mail", "old@x.com", "new@x.com", false, "LDAP modify failed")

	require.Len(t, ml.entries, 1)
	e := ml.entries[0]
	assert.Equal(t, audit.StatusFailure, e.Status)
	assert.Contains(t, e.Details, "LDAP modify failed")
}

func TestLogApplyMarkdown_Summary(t *testing.T) {
	ml := &captureLogger{}
	audit.Init(ml)
	t.Cleanup(func() { audit.Init(nil) })

	changes := []audit.OperationChange{
		{DN: "CN=A", Attribute: "mail", OldValue: "old@x.com", NewValue: "new@x.com", Success: true},
		{DN: "CN=B", Attribute: "mail", OldValue: "b@x.com", NewValue: "fail@x.com", Success: false, Error: "not found"},
	}
	audit.LogApplyMarkdown("editor", "10.0.0.3", "# Employee Data Corrections\n", changes)

	require.Len(t, ml.entries, 1)
	e := ml.entries[0]
	assert.Equal(t, audit.ActionApplyMarkdown, e.Action)
	assert.Equal(t, audit.StatusFailure, e.Status, "at least one failure → overall failure")
	assert.Contains(t, e.Details, "operations_applied")
	assert.Contains(t, e.Details, "operations_failed")
	assert.Equal(t, "editor", e.Operator)
}

func TestLogApplyMarkdown_AllSuccess(t *testing.T) {
	ml := &captureLogger{}
	audit.Init(ml)
	t.Cleanup(func() { audit.Init(nil) })

	changes := []audit.OperationChange{
		{DN: "CN=A", Attribute: "mail", Success: true},
	}
	audit.LogApplyMarkdown("ed", "127.0.0.1", "", changes)

	require.Len(t, ml.entries, 1)
	assert.Equal(t, audit.StatusSuccess, ml.entries[0].Status)
}

func TestEmit_NilLogger_NoopNotPanic(t *testing.T) {
	audit.Init(nil)
	// Must not panic.
	assert.NotPanics(t, func() {
		audit.LogLogin("user", "127.0.0.1", true, "")
	})
}

// ── ClientIPFromRequest ────────────────────────────────────────────────────────

func TestClientIPFromRequest_XForwardedFor(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "192.168.1.1, 10.0.0.1")
	assert.Equal(t, "192.168.1.1", audit.ClientIPFromRequest(req))
}

func TestClientIPFromRequest_XRealIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Real-IP", "172.16.0.5")
	assert.Equal(t, "172.16.0.5", audit.ClientIPFromRequest(req))
}

func TestClientIPFromRequest_RemoteAddr(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "203.0.113.7:4321"
	assert.Equal(t, "203.0.113.7", audit.ClientIPFromRequest(req))
}

func TestClientIPFromRequest_XForwardedFor_TakesPrecedence(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.Header.Set("X-Real-IP", "5.6.7.8")
	assert.Equal(t, "1.2.3.4", audit.ClientIPFromRequest(req))
}
