package markdown

import "ad-sync-manager/internal/employee"

// ADClient is the interface the markdown correction system uses to interact with
// Active Directory. The default implementation wraps the employee package
// functions; tests use a mock.
type ADClient interface {
	GetEmployeeByDN(dn string) (*employee.Employee, error)
	UpdateEmployeeAttribute(dn, attrName, newValue string) error
}

// MarkdownOperation represents one attribute-correction extracted from the
// correction document.
type MarkdownOperation struct {
	DN        string `json:"dn"`
	Attribute string `json:"attribute"`
	NewValue  string `json:"newValue"`
	OldValue  string `json:"oldValue"`
	Valid     bool   `json:"valid"`
	Error     string `json:"error,omitempty"`
}

// ValidateResponse is returned by POST /api/v1/markdown/validate.
type ValidateResponse struct {
	Valid      bool                `json:"valid"`
	Operations []MarkdownOperation `json:"operations"`
	Errors     []string            `json:"errors"`
}

// ApplyResponse is returned by POST /api/v1/markdown/apply.
type ApplyResponse struct {
	Applied int               `json:"applied"`
	Failed  int               `json:"failed"`
	Details []OperationResult `json:"details"`
}

// OperationResult summarises the outcome of a single apply attempt.
type OperationResult struct {
	DN        string `json:"dn"`
	Attribute string `json:"attribute"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
}
