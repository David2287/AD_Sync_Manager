package http

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"ad-sync-manager/internal/audit"
)

// LogsHandler serves the audit log API endpoints.
// All routes must be protected by RequireAdmin middleware (enforced at the router).
type LogsHandler struct {
	querier audit.AuditQuerier
}

// NewLogsHandler returns a handler backed by the given querier.
// If querier is nil, every request receives 503 Service Unavailable.
func NewLogsHandler(q audit.AuditQuerier) *LogsHandler {
	return &LogsHandler{querier: q}
}

// List handles GET /api/v1/logs.
//
// Query parameters (all optional):
//
//	limit    int    — max entries to return; default 50, max 200
//	offset   int    — records to skip; default 0
//	from     string — ISO 8601 lower bound (inclusive) on timestamp
//	to       string — ISO 8601 upper bound (inclusive) on timestamp
//	operator string — filter by sAMAccountName
//	action   string — filter by action (login|apply_markdown|update_employee|integrity_check)
//	status   string — filter by status (success|failure)
//
// Response: {"data": [...], "total": N, "limit": N, "offset": N}
func (h *LogsHandler) List(c *gin.Context) {
	if h.querier == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "audit database not configured"})
		return
	}

	f := audit.LogFilter{
		Operator: c.Query("operator"),
		Action:   c.Query("action"),
		Status:   c.Query("status"),
		Limit:    queryInt(c, "limit", 50),
		Offset:   queryInt(c, "offset", 0),
	}
	if f.Limit > 200 {
		f.Limit = 200
	}
	if raw := c.Query("from"); raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			f.From = &t
		}
	}
	if raw := c.Query("to"); raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			f.To = &t
		}
	}

	entries, total, err := h.querier.List(c.Request.Context(), f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query audit logs"})
		return
	}
	if entries == nil {
		entries = []audit.AuditLog{}
	}

	c.JSON(http.StatusOK, gin.H{
		"data":   entries,
		"total":  total,
		"limit":  f.Limit,
		"offset": f.Offset,
	})
}

// GetByID handles GET /api/v1/logs/:id.
// Returns the single audit entry with the given database id.
func (h *LogsHandler) GetByID(c *gin.Context) {
	if h.querier == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "audit database not configured"})
		return
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id must be an integer"})
		return
	}

	entry, err := h.querier.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve audit log entry"})
		return
	}
	if entry == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audit log entry not found"})
		return
	}

	c.JSON(http.StatusOK, entry)
}

func queryInt(c *gin.Context, key string, fallback int) int {
	if raw := c.Query(key); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			return v
		}
	}
	return fallback
}
