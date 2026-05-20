package markdown

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"ad-sync-manager/internal/audit"
	"ad-sync-manager/internal/auth"
	"ad-sync-manager/internal/employee"
)

// pkgADClient is the package-level ADClient set by Init.
// Kept for backward-compat; handlers now create context-aware clients per request.
var pkgADClient ADClient

// Init sets a fallback ADClient. Kept for backward-compat and tests.
func Init(client ADClient) {
	pkgADClient = client
}

// NewEmployeeADClient returns the default ADClient backed by the employee package
// using context.Background() — suitable for tests or when no user context exists.
func NewEmployeeADClient() ADClient {
	return contextAwareAdapter{ctx: context.Background()}
}

// contextAwareAdapter implements ADClient and threads the request context into
// every employee package call so that LDAP binds respect the per-user credentials
// injected by RequireAuth middleware when AD_USE_USER_BIND=true.
type contextAwareAdapter struct {
	ctx context.Context
}

func (a contextAwareAdapter) GetEmployeeByDN(dn string) (*employee.Employee, error) {
	return employee.GetEmployeeByDN(a.ctx, dn)
}

func (a contextAwareAdapter) UpdateEmployeeAttribute(dn, attrName, newValue string) error {
	return employee.UpdateEmployeeAttribute(a.ctx, dn, attrName, newValue)
}

// newRequestADClient creates a context-aware ADClient for the current HTTP
// request. When AD_USE_USER_BIND=true the context carries the user's LDAPCred
// and all AD operations bind as that user; otherwise they fall back to the
// service account transparently.
func newRequestADClient(r *http.Request) ADClient {
	return contextAwareAdapter{ctx: r.Context()}
}

// ValidateMarkdownHandler handles POST /api/v1/markdown/validate.
//
// Accepts JSON: {"markdown": "..."}
// Parses the document, validates every operation against AD, and returns a
// structured response. Never writes to AD.
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

	ops, errs := ValidateOperations(ops, newRequestADClient(r))

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

	client := newRequestADClient(r)
	ops, _ = ValidateOperations(ops, client)
	details := ApplyOperations(ops, client)

	var applied, failed int
	for _, d := range details {
		if d.Success {
			applied++
		} else {
			failed++
		}
	}

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
