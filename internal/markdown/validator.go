package markdown

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"ad-sync-manager/internal/employee"
)

// allowedAttrs is the set of LDAP attribute names the markdown correction
// workflow may modify. It is broader than the employee REST API allowlist
// (telephoneNumber + physicalDeliveryOfficeName) because batch corrections made
// by support technicians include display name and e-mail fixes as well.
//
// TODO(Stage 5): expose this set via an admin endpoint so it can be managed
// without a redeploy.
var allowedAttrs = map[string]bool{
	"telephoneNumber":            true,
	"physicalDeliveryOfficeName": true,
	"mail":                       true,
	"displayName":                true,
}

// ValidateOperations checks each operation for semantic validity:
//   - Attribute name is on the allowlist.
//   - New value is non-empty.
//   - Employee DN exists in AD; on success, OldValue is populated.
//
// All operations are returned (both valid and invalid) so callers can present a
// complete audit view. errMsgs collects one human-readable message per invalid
// operation.
//
// NOTE: For large documents (100+ operations) each validation round-trip hits
// AD. Consider batching DN lookups or adding a per-request timeout context in
// a future performance pass.
func ValidateOperations(ops []MarkdownOperation, adClient ADClient) ([]MarkdownOperation, []string) {
	var errMsgs []string
	result := make([]MarkdownOperation, 0, len(ops))

	for i, op := range ops {
		op.Valid = true

		normalized := normalizeAttr(op.Attribute)
		if normalized == "" {
			op.Valid = false
			op.Error = fmt.Sprintf("operation %d: unknown attribute %q", i+1, op.Attribute)
			errMsgs = append(errMsgs, op.Error)
			result = append(result, op)
			continue
		}
		op.Attribute = normalized

		if strings.TrimSpace(op.NewValue) == "" {
			op.Valid = false
			op.Error = fmt.Sprintf("operation %d: new value must not be empty", i+1)
			errMsgs = append(errMsgs, op.Error)
			result = append(result, op)
			continue
		}

		emp, err := adClient.GetEmployeeByDN(op.DN)
		if err != nil {
			op.Valid = false
			if errors.Is(err, employee.ErrNotFound) {
				op.Error = fmt.Sprintf("operation %d: employee not found: %s", i+1, op.DN)
			} else {
				op.Error = fmt.Sprintf("operation %d: failed to look up employee", i+1)
			}
			errMsgs = append(errMsgs, op.Error)
			result = append(result, op)
			continue
		}

		op.OldValue = attrValueFrom(emp, op.Attribute)
		result = append(result, op)
	}

	return result, errMsgs
}

// ApplyOperations calls UpdateEmployeeAttribute for each valid operation.
// Invalid operations (Valid == false) are recorded as failures without touching
// AD. Partial success is allowed — a failure on one operation does not halt the
// rest.
//
// Placeholder log.Printf calls here will be replaced by structured audit logging
// in Stage 5.
func ApplyOperations(ops []MarkdownOperation, adClient ADClient) []OperationResult {
	results := make([]OperationResult, 0, len(ops))
	for _, op := range ops {
		r := OperationResult{DN: op.DN, Attribute: op.Attribute}
		if !op.Valid {
			r.Error = op.Error
			results = append(results, r)
			continue
		}
		if err := adClient.UpdateEmployeeAttribute(op.DN, op.Attribute, op.NewValue); err != nil {
			r.Error = "update failed"
			log.Printf("markdown apply FAILED: dn=%q attr=%q error=%v", op.DN, op.Attribute, err)
		} else {
			r.Success = true
			log.Printf("markdown apply OK: dn=%q attr=%q old=%q new=%q",
				op.DN, op.Attribute, op.OldValue, op.NewValue)
		}
		results = append(results, r)
	}
	return results
}

// normalizeAttr maps a user-supplied attribute name (case-insensitive) to its
// canonical LDAP form, or returns "" if it is not on the allowlist.
func normalizeAttr(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "telephonenumber":
		return "telephoneNumber"
	case "physicaldeliveryofficename":
		return "physicalDeliveryOfficeName"
	case "mail":
		return "mail"
	case "displayname":
		return "displayName"
	default:
		return ""
	}
}

// attrValueFrom returns the current value of the given LDAP attribute from an
// Employee snapshot. Used to populate OldValue during validation.
func attrValueFrom(emp *employee.Employee, attr string) string {
	switch attr {
	case "telephoneNumber":
		return emp.TelephoneNumber
	case "physicalDeliveryOfficeName":
		return emp.Office
	case "mail":
		return emp.Email
	case "displayName":
		return emp.FullName
	default:
		return ""
	}
}
