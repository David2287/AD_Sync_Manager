package http

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"ad-sync-manager/internal/domain/interfaces"
)

// SyncHandler exposes the AD sync trigger and status endpoints.
type SyncHandler struct {
	svc interfaces.SyncService
}

func NewSyncHandler(svc interfaces.SyncService) *SyncHandler {
	return &SyncHandler{svc: svc}
}

// Run godoc
// POST /api/v1/sync   [admin]
// Triggers an immediate full sync and blocks until it completes.
func (h *SyncHandler) Run(c *gin.Context) {
	result, err := h.svc.Run(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// Status godoc
// GET /api/v1/sync/status   [admin]
func (h *SyncHandler) Status(c *gin.Context) {
	result, err := h.svc.LastResult(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if result == nil {
		c.JSON(http.StatusOK, gin.H{"data": nil, "message": "no sync has run yet"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}
