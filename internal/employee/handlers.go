package employee

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"ad-sync-manager/internal/audit"
	"ad-sync-manager/internal/auth"
)

const cacheTTL = 5 * time.Minute

var (
	pkgCache = newCache()

	// listVersion is incremented on every successful PUT update.
	// All list cache keys embed the current version, so bumping it makes every
	// existing list entry unreachable without explicit deletion — they expire
	// naturally via TTL.
	listVersion int64
)

// listResult is the JSON envelope returned by ListEmployeesHandler.
type listResult struct {
	Data   []Employee `json:"data"`
	Total  int        `json:"total"`
	Limit  int        `json:"limit"`
	Offset int        `json:"offset"`
}

// ListEmployeesHandler handles GET /api/v1/employees.
//
// Query parameters:
//
//	limit  – max records to return, clamped to [1, 200]; default 50
//	offset – records to skip; default 0
//	search – optional substring matched against displayName and mail
//
// Responses are cached for cacheTTL (5 min). The cache is invalidated globally
// on any successful PUT by incrementing listVersion.
func ListEmployeesHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit  := clampInt(parseIntParam(q.Get("limit"), 50), 1, 200)
	offset := max0Int(parseIntParam(q.Get("offset"), 0))
	search := q.Get("search")

	filter   := buildListFilter(search)
	cacheKey := fmt.Sprintf("list:v%d:%s:%d:%d",
		atomic.LoadInt64(&listVersion), filter, limit, offset)

	if cached := pkgCache.Get(cacheKey); cached != nil {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	employees, total, err := GetAllEmployees(r.Context(), adCfg.EmployeeOU, filter, limit, offset)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to list employees")
		return
	}

	result := listResult{Data: employees, Total: total, Limit: limit, Offset: offset}
	pkgCache.Set(cacheKey, result, cacheTTL)
	writeJSON(w, http.StatusOK, result)
}

// GetEmployeeHandler handles GET /api/v1/employees/:dn.
//
// The DN is read from the "dn" URL query parameter. The Gin router injects it
// from the :dn path segment before delegating to this handler (see router.go).
// Individual records are cached for cacheTTL and invalidated on PUT.
func GetEmployeeHandler(w http.ResponseWriter, r *http.Request) {
	dn := r.URL.Query().Get("dn")
	if dn == "" {
		writeJSONError(w, http.StatusBadRequest, "dn query parameter is required")
		return
	}

	cacheKey := "employee:" + dn
	if cached := pkgCache.Get(cacheKey); cached != nil {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	emp, err := GetEmployeeByDN(r.Context(), dn)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "employee not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to get employee")
		return
	}

	pkgCache.Set(cacheKey, emp, cacheTTL)
	writeJSON(w, http.StatusOK, emp)
}

// UpdateEmployeeHandler handles PUT /api/v1/employees/:dn.
//
// The DN is read from the "dn" URL query parameter (injected by the Gin router).
// Group membership enforcement (RequireGroup) is applied by the router middleware,
// not by this handler.
//
// Accepted JSON body fields:
//
//	telephoneNumber            – replaces the AD telephoneNumber attribute
//	physicalDeliveryOfficeName – replaces the AD physicalDeliveryOfficeName attribute
//
// Only non-empty fields are written; omitted fields are left unchanged.
// Returns the updated employee object on success.
func UpdateEmployeeHandler(w http.ResponseWriter, r *http.Request) {
	dn := r.URL.Query().Get("dn")
	if dn == "" {
		writeJSONError(w, http.StatusBadRequest, "dn query parameter is required")
		return
	}

	var body struct {
		TelephoneNumber            string `json:"telephoneNumber"`
		PhysicalDeliveryOfficeName string `json:"physicalDeliveryOfficeName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Extract operator and IP for audit logging.
	var operator string
	if claims := auth.ClaimsFromContext(r.Context()); claims != nil {
		operator = claims.Username
	}
	ip := audit.ClientIPFromRequest(r)

	// Fetch current attribute values before mutating so we can log old → new.
	// Best-effort: if the lookup fails we log with empty old values and continue.
	var oldPhone, oldOffice string
	if old, err := GetEmployeeByDN(r.Context(), dn); err == nil {
		oldPhone = old.TelephoneNumber
		oldOffice = old.Office
	}

	updates := []struct{ attr, val, oldVal string }{
		{"telephoneNumber", body.TelephoneNumber, oldPhone},
		{"physicalDeliveryOfficeName", body.PhysicalDeliveryOfficeName, oldOffice},
	}
	for _, u := range updates {
		if u.val == "" {
			continue
		}
		updateErr := UpdateEmployeeAttribute(r.Context(), dn, u.attr, u.val)
		var errMsg string
		if updateErr != nil {
			errMsg = updateErr.Error()
		}
		audit.LogUpdateEmployee(operator, ip, dn, u.attr, u.oldVal, u.val, updateErr == nil, errMsg)
		if updateErr != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to update employee")
			return
		}
	}

	// Invalidate the per-employee cache entry and all list pages.
	pkgCache.Delete("employee:" + dn)
	atomic.AddInt64(&listVersion, 1)

	emp, err := GetEmployeeByDN(r.Context(), dn)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "employee not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve updated employee")
		return
	}

	writeJSON(w, http.StatusOK, emp)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"error":%q}`, msg)
}

func parseIntParam(s string, fallback int) int {
	if s == "" {
		return fallback
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return v
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func max0Int(v int) int {
	if v < 0 {
		return 0
	}
	return v
}
