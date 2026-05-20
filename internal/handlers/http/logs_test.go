package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ad-sync-manager/internal/audit"
	httphandlers "ad-sync-manager/internal/handlers/http"
)

// ── mock AuditQuerier ─────────────────────────────────────────────────────────

type mockQuerier struct {
	entries []audit.AuditLog
	err     error
}

func (m *mockQuerier) List(_ context.Context, f audit.LogFilter) ([]audit.AuditLog, int, error) {
	if m.err != nil {
		return nil, 0, m.err
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > len(m.entries) {
		limit = len(m.entries)
	}
	return m.entries[:limit], len(m.entries), nil
}

func (m *mockQuerier) GetByID(_ context.Context, id int) (*audit.AuditLog, error) {
	if m.err != nil {
		return nil, m.err
	}
	for i := range m.entries {
		if m.entries[i].ID == id {
			return &m.entries[i], nil
		}
	}
	return nil, nil
}

// newLogsRouter wires a LogsHandler into a minimal Gin router.
func newLogsRouter(q audit.AuditQuerier) *gin.Engine {
	r := gin.New()
	h := httphandlers.NewLogsHandler(q)
	r.GET("/logs", h.List)
	r.GET("/logs/:id", h.GetByID)
	return r
}

// ── nil querier ───────────────────────────────────────────────────────────────

func TestLogsHandler_NilQuerier_List_Returns503(t *testing.T) {
	r := newLogsRouter(nil)
	req := httptest.NewRequest(http.MethodGet, "/logs", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), "error")
}

func TestLogsHandler_NilQuerier_GetByID_Returns503(t *testing.T) {
	r := newLogsRouter(nil)
	req := httptest.NewRequest(http.MethodGet, "/logs/1", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// ── List ──────────────────────────────────────────────────────────────────────

func TestLogsHandler_List_ReturnsEntries(t *testing.T) {
	q := &mockQuerier{entries: []audit.AuditLog{
		{ID: 1, Operator: "alice", Action: audit.ActionLogin, Status: audit.StatusSuccess, Timestamp: time.Now()},
		{ID: 2, Operator: "bob", Action: audit.ActionUpdateEmployee, Status: audit.StatusSuccess, Timestamp: time.Now()},
	}}
	r := newLogsRouter(q)
	req := httptest.NewRequest(http.MethodGet, "/logs", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body struct {
		Data   []audit.AuditLog `json:"data"`
		Total  int              `json:"total"`
		Limit  int              `json:"limit"`
		Offset int              `json:"offset"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, 2, body.Total)
	assert.Len(t, body.Data, 2)
	assert.Equal(t, 50, body.Limit)
	assert.Equal(t, 0, body.Offset)
}

func TestLogsHandler_List_EmptyResultIsNotNull(t *testing.T) {
	r := newLogsRouter(&mockQuerier{})
	req := httptest.NewRequest(http.MethodGet, "/logs", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"data":[]`)
}

func TestLogsHandler_List_LimitCappedAt200(t *testing.T) {
	// Create 5 entries and ask for 999 — response limit must be 200 max.
	entries := make([]audit.AuditLog, 5)
	for i := range entries {
		entries[i] = audit.AuditLog{ID: i + 1, Action: audit.ActionLogin, Status: audit.StatusSuccess}
	}
	r := newLogsRouter(&mockQuerier{entries: entries})
	req := httptest.NewRequest(http.MethodGet, "/logs?limit=999", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Limit int `json:"limit"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, 200, body.Limit)
}

func TestLogsHandler_List_PaginationParams(t *testing.T) {
	r := newLogsRouter(&mockQuerier{})
	req := httptest.NewRequest(http.MethodGet, "/logs?limit=10&offset=20", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, 10, body.Limit)
	assert.Equal(t, 20, body.Offset)
}

// ── GetByID ───────────────────────────────────────────────────────────────────

func TestLogsHandler_GetByID_Found(t *testing.T) {
	q := &mockQuerier{entries: []audit.AuditLog{
		{ID: 42, Operator: "jdoe", Action: audit.ActionLogin, Status: audit.StatusSuccess, Timestamp: time.Now()},
	}}
	r := newLogsRouter(q)
	req := httptest.NewRequest(http.MethodGet, "/logs/42", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var entry audit.AuditLog
	require.NoError(t, json.NewDecoder(w.Body).Decode(&entry))
	assert.Equal(t, 42, entry.ID)
	assert.Equal(t, "jdoe", entry.Operator)
}

func TestLogsHandler_GetByID_NotFound(t *testing.T) {
	r := newLogsRouter(&mockQuerier{})
	req := httptest.NewRequest(http.MethodGet, "/logs/9999", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestLogsHandler_GetByID_InvalidID(t *testing.T) {
	r := newLogsRouter(&mockQuerier{})
	req := httptest.NewRequest(http.MethodGet, "/logs/not-an-int", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "error")
}
