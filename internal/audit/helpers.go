package audit

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// pkgLogger is the package-level audit logger set by Init.
// All helper functions write through this logger; a nil logger is a no-op.
var pkgLogger AuditLogger

// Init sets the package-level audit logger used by LogLogin, LogUpdateEmployee,
// and LogApplyMarkdown. Must be called once at application startup, before the
// HTTP server begins accepting requests.
func Init(l AuditLogger) { pkgLogger = l }

// ClientIPFromRequest extracts the real client IP from an HTTP request.
// It checks the X-Forwarded-For and X-Real-IP proxy headers before falling
// back to r.RemoteAddr, which may be the address of a load balancer.
func ClientIPFromRequest(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		return strings.TrimSpace(strings.SplitN(fwd, ",", 2)[0])
	}
	if real := r.Header.Get("X-Real-IP"); real != "" {
		return strings.TrimSpace(real)
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// LogLogin records the outcome of a login attempt.
// Call this after every AD authentication attempt, regardless of outcome.
// Passwords must never be passed to this function.
func LogLogin(operator, ip string, success bool, reason string) {
	status := StatusSuccess
	if !success {
		status = StatusFailure
	}
	entry := AuditLog{
		Operator:  operator,
		Action:    ActionLogin,
		Status:    status,
		IPAddress: ip,
	}
	if reason != "" {
		entry.Details = fmt.Sprintf(`{"reason":%q}`, reason)
	}
	emit(entry)
}

// LogUpdateEmployee records a single attribute modification on an AD employee.
// Log one call per changed attribute, not one per request.
func LogUpdateEmployee(operator, ip, dn, attribute, oldValue, newValue string, success bool, errMsg string) {
	status := StatusSuccess
	if !success {
		status = StatusFailure
	}
	entry := AuditLog{
		Operator:  operator,
		Action:    ActionUpdateEmployee,
		TargetDN:  dn,
		Attribute: attribute,
		OldValue:  oldValue,
		NewValue:  newValue,
		Status:    status,
		IPAddress: ip,
	}
	if errMsg != "" {
		entry.Details = fmt.Sprintf(`{"error":%q}`, errMsg)
	}
	emit(entry)
}

// LogApplyMarkdown records the result of a Markdown correction apply run.
// One audit entry is written per apply request; the per-change breakdown and
// the original Markdown text are embedded as JSON in the Details field.
func LogApplyMarkdown(operator, ip, markdownText string, changes []OperationChange) {
	applied, failed := 0, 0
	for _, ch := range changes {
		if ch.Success {
			applied++
		} else {
			failed++
		}
	}

	type payload struct {
		OperationsApplied int               `json:"operations_applied"`
		OperationsFailed  int               `json:"operations_failed"`
		Changes           []OperationChange `json:"changes"`
		Markdown          string            `json:"markdown,omitempty"`
	}
	b, _ := json.Marshal(payload{
		OperationsApplied: applied,
		OperationsFailed:  failed,
		Changes:           changes,
		Markdown:          markdownText,
	})

	status := StatusSuccess
	if failed > 0 {
		status = StatusFailure
	}

	emit(AuditLog{
		Operator:  operator,
		Action:    ActionApplyMarkdown,
		Status:    status,
		Details:   string(b),
		IPAddress: ip,
	})
}

// emit stamps entry with the current UTC time and forwards it to pkgLogger.
// Logging failures are written to stderr so they never surface to the caller.
func emit(entry AuditLog) {
	if pkgLogger == nil {
		return
	}
	entry.Timestamp = time.Now().UTC()
	if err := pkgLogger.Log(entry); err != nil {
		fmt.Fprintf(os.Stderr, "audit: failed to emit %s event: %v\n", entry.Action, err)
	}
}
