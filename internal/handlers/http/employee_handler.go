package http

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"ad-sync-manager/internal/domain/interfaces"
)

// EmployeeHandler serves employee read endpoints.
type EmployeeHandler struct {
	svc interfaces.EmployeeService
}

func NewEmployeeHandler(svc interfaces.EmployeeService) *EmployeeHandler {
	return &EmployeeHandler{svc: svc}
}

// List godoc
// GET /api/v1/employees?office=HQ&search=john&page=1&page_size=50
func (h *EmployeeHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))

	filter := interfaces.EmployeeFilter{
		Office:   c.Query("office"),
		Search:   c.Query("search"),
		Page:     page,
		PageSize: pageSize,
	}

	employees, err := h.svc.List(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list employees"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": employees, "count": len(employees)})
}

// Get godoc
// GET /api/v1/employees/:id
func (h *EmployeeHandler) Get(c *gin.Context) {
	emp, err := h.svc.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "employee not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": emp})
}
