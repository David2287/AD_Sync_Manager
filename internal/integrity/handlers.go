package integrity

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// IntegrityHandler serves the integrity report and baseline reset endpoints.
// All routes must be protected by RequireAdmin middleware (enforced at the router).
type IntegrityHandler struct {
	checker *Checker
}

// NewIntegrityHandler wraps a Checker for use as Gin route handlers.
func NewIntegrityHandler(checker *Checker) *IntegrityHandler {
	return &IntegrityHandler{checker: checker}
}

// GetReport handles GET /api/v1/integrity/report.
//
// Returns the mismatch list from the most recently completed periodic check.
// An empty list means no violations have been found since the last run.
//
// Response: {"mismatches": [...], "count": N}
func (h *IntegrityHandler) GetReport(c *gin.Context) {
	mismatches := h.checker.LastResult()
	c.JSON(http.StatusOK, gin.H{
		"mismatches": mismatches,
		"count":      len(mismatches),
	})
}

// ResetBaseline handles POST /api/v1/integrity/reset.
//
// Fetches current AD state, reports any deviations from the stored baseline,
// then overwrites every baseline entry with the current hash. Use this after a
// bulk legitimate update to acknowledge the changes and prevent false positives
// on the next scheduled check.
//
// Response: {"total_employees": N, "mismatches_found": N, "baseline_updated": true}
func (h *IntegrityHandler) ResetBaseline(c *gin.Context) {
	mismatches, total, err := h.checker.Reset(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"total_employees":  total,
		"mismatches_found": len(mismatches),
		"baseline_updated": true,
	})
}
