package markdown

import (
	"encoding/json"
	"fmt"
	"net/http"

	"ad-sync-manager/internal/audit"
	"ad-sync-manager/internal/auth"
	"ad-sync-manager/internal/employee"
)

// pkgADClient is the package-level ADClient set by Init.
var pkgADClient ADClient

// Init sets the ADClient used by all markdown handlers. Must be called once at
// application startup, before any HTTP request reaches these handlers.
func Init(client ADClient) {
	pkgADClient = client
}

// NewEmployeeADClient returns the default ADClient backed by the employee package.
// Pass this to Init in production; use a mock in tests.
func NewEmployeeADClient() ADClient {
	return employeeAdapter{}
}

// employeeAdapter adapts the package-level employee functions to the ADClient
// interface so the markdown package has no compile-time dependency on how the
// employee package is initialised.
type employeeAdapter struct{}

func (employeeAdapter) GetEmployeeByDN(dn string) (*employee.Employee, error) {
	return employee.GetEmployeeByDN(dn)
}

func (employeeAdapter) UpdateEmployeeAttribute(dn, attrName, newValue string) error {
	return employee.UpdateEmployeeAttribute(dn, attrName, newValue)
}

// ValidateMarkdownHandler handles POST /api/v1/markdown/validate.
//
// Accepts JSON: {"markdown": "..."}
// Parses the document, validates every operation against AD, and returns a
// structured response. Never writes to AD.
//
// HTTP status codes:
//
//	400 – invalid JSON body or completely unparseable document
//	200 – document parsed; valid:false when semantic errors were found
func ValidateMarkdownHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Markdown string `json:"markdown"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	ops, err := ParseMarkdown(req.Markdown)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	ops, errs := ValidateOperations(ops, pkgADClient)

	if ops == nil {
		ops = []MarkdownOperation{}
	}
	if errs == nil {
		errs = []string{}
	}

	writeJSON(w, http.StatusOK, ValidateResponse{
		Valid:      len(errs) == 0,
		Operations: ops,
		Errors:     errs,
	})
}

// ApplyMarkdownHandler handles POST /api/v1/markdown/apply.
//
// Requires RequireGroup middleware (EditorGroupDN) applied at the router level.
// Parses, validates, and applies all valid operations. Partial success is allowed.
//
// HTTP status codes:
//
//	400 – invalid JSON body or completely unparseable document
//	200 – processing complete (check applied/failed counts in body)
func ApplyMarkdownHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Markdown string `json:"markdown"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	ops, err := ParseMarkdown(req.Markdown)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	ops, _ = ValidateOperations(ops, pkgADClient)
	details := ApplyOperations(ops, pkgADClient)

	var applied, failed int
	for _, d := range details {
		if d.Success {
			applied++
		} else {
			failed++
		}
	}

	// Build audit change list correlating validated ops (old/new values) with
	// apply results (success flag). Both slices are parallel by construction.
	var operator string
	if claims := auth.ClaimsFromContext(r.Context()); claims != nil {
		operator = claims.Username
	}
	ip := audit.ClientIPFromRequest(r)
	changes := make([]audit.OperationChange, len(details))
	for i, d := range details {
		changes[i] = audit.OperationChange{
			DN:        d.DN,
			Attribute: d.Attribute,
			Success:   d.Success,
			Error:     d.Error,
		}
		if i < len(ops) {
			changes[i].OldValue = ops[i].OldValue
			changes[i].NewValue = ops[i].NewValue
		}
	}
	audit.LogApplyMarkdown(operator, ip, req.Markdown, changes)

	if details == nil {
		details = []OperationResult{}
	}

	writeJSON(w, http.StatusOK, ApplyResponse{
		Applied: applied,
		Failed:  failed,
		Details: details,
	})
}

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
