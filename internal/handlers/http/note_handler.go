package http

import (
	"net/http"

	"github.com/gin-gonic/gin"

	mw "ad-sync-manager/internal/middleware"
	"ad-sync-manager/internal/domain/interfaces"
)

// NoteHandler serves markdown note CRUD endpoints.
type NoteHandler struct {
	svc interfaces.NoteService
}

func NewNoteHandler(svc interfaces.NoteService) *NoteHandler {
	return &NoteHandler{svc: svc}
}

type noteBody struct {
	Content string `json:"content" binding:"required"`
}

// ListForEmployee godoc
// GET /api/v1/employees/:id/notes
func (h *NoteHandler) ListForEmployee(c *gin.Context) {
	notes, err := h.svc.ListForEmployee(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list notes"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": notes})
}

// Create godoc
// POST /api/v1/employees/:id/notes
func (h *NoteHandler) Create(c *gin.Context) {
	var body noteBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sess := mw.SessionFrom(c)
	note, err := h.svc.Create(c.Request.Context(), c.Param("id"), sess.UserID, body.Content)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": note})
}

// Update godoc
// PUT /api/v1/notes/:id
func (h *NoteHandler) Update(c *gin.Context) {
	var body noteBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sess := mw.SessionFrom(c)
	note, err := h.svc.Update(c.Request.Context(), c.Param("id"), sess.UserID, body.Content)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": note})
}

// Delete godoc
// DELETE /api/v1/notes/:id
func (h *NoteHandler) Delete(c *gin.Context) {
	sess := mw.SessionFrom(c)
	if err := h.svc.Delete(c.Request.Context(), c.Param("id"), sess.UserID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
