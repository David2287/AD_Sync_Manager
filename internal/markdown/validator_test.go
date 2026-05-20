package markdown

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ad-sync-manager/internal/employee"
)

// ── Mock ADClient ─────────────────────────────────────────────────────────────

type mockADClient struct {
	employees map[string]*employee.Employee
	updateErr map[string]error // key: dn+":"+attr
}

func newMockADClient() *mockADClient {
	return &mockADClient{
		employees: make(map[string]*employee.Employee),
		updateErr: make(map[string]error),
	}
}

func (m *mockADClient) addEmployee(e *employee.Employee) {
	m.employees[e.DN] = e
}

func (m *mockADClient) setUpdateErr(dn, attr string, err error) {
	m.updateErr[dn+":"+attr] = err
}

func (m *mockADClient) GetEmployeeByDN(dn string) (*employee.Employee, error) {
	e, ok := m.employees[dn]
	if !ok {
		return nil, employee.ErrNotFound
	}
	return e, nil
}

func (m *mockADClient) UpdateEmployeeAttribute(dn, attrName, _ string) error {
	if err, ok := m.updateErr[dn+":"+attrName]; ok {
		return err
	}
	return nil
}

// ── ValidateOperations ────────────────────────────────────────────────────────

func TestValidateOperations_AllValid(t *testing.T) {
	cli := newMockADClient()
	cli.addEmployee(&employee.Employee{
		DN:              "CN=John,OU=Employees,DC=company,DC=com",
		TelephoneNumber: "old-phone",
	})

	ops := []MarkdownOperation{
		{DN: "CN=John,OU=Employees,DC=company,DC=com", Attribute: "telephoneNumber", NewValue: "+7 999 000 00 00"},
	}
	result, errs := ValidateOperations(ops, cli)

	require.Empty(t, errs)
	require.Len(t, result, 1)
	assert.True(t, result[0].Valid)
	assert.Equal(t, "old-phone", result[0].OldValue)
	assert.Equal(t, "telephoneNumber", result[0].Attribute)
}

func TestValidateOperations_UnknownAttribute(t *testing.T) {
	cli := newMockADClient()
	ops := []MarkdownOperation{
		{DN: "CN=X,DC=y,DC=com", Attribute: "invalidAttr", NewValue: "val"},
	}
	result, errs := ValidateOperations(ops, cli)

	require.Len(t, errs, 1)
	assert.Contains(t, errs[0], "unknown attribute")
	assert.False(t, result[0].Valid)
}

func TestValidateOperations_EmployeeNotFound(t *testing.T) {
	cli := newMockADClient()
	ops := []MarkdownOperation{
		{DN: "CN=Ghost,DC=y,DC=com", Attribute: "telephoneNumber", NewValue: "123"},
	}
	result, errs := ValidateOperations(ops, cli)

	require.Len(t, errs, 1)
	assert.Contains(t, errs[0], "employee not found")
	assert.False(t, result[0].Valid)
}

func TestValidateOperations_EmptyNewValue(t *testing.T) {
	cli := newMockADClient()
	ops := []MarkdownOperation{
		{DN: "CN=X,DC=y,DC=com", Attribute: "mail", NewValue: ""},
	}
	result, errs := ValidateOperations(ops, cli)

	require.Len(t, errs, 1)
	assert.Contains(t, errs[0], "new value must not be empty")
	assert.False(t, result[0].Valid)
}

func TestValidateOperations_CaseInsensitiveAttributeNormalized(t *testing.T) {
	cli := newMockADClient()
	cli.addEmployee(&employee.Employee{DN: "CN=A,DC=b,DC=com", Email: "old@x.com"})

	ops := []MarkdownOperation{
		{DN: "CN=A,DC=b,DC=com", Attribute: "MAIL", NewValue: "new@x.com"},
	}
	result, errs := ValidateOperations(ops, cli)

	require.Empty(t, errs)
	assert.True(t, result[0].Valid)
	assert.Equal(t, "mail", result[0].Attribute)
	assert.Equal(t, "old@x.com", result[0].OldValue)
}

func TestValidateOperations_OldValuePopulatedForAllAllowedAttrs(t *testing.T) {
	cli := newMockADClient()
	cli.addEmployee(&employee.Employee{
		DN:              "CN=E,DC=x,DC=com",
		TelephoneNumber: "t",
		Office:          "o",
		Email:           "e@x.com",
		FullName:        "Full Name",
	})

	cases := []struct {
		attr     string
		expected string
	}{
		{"telephoneNumber", "t"},
		{"physicalDeliveryOfficeName", "o"},
		{"mail", "e@x.com"},
		{"displayName", "Full Name"},
	}

	for _, tc := range cases {
		ops := []MarkdownOperation{
			{DN: "CN=E,DC=x,DC=com", Attribute: tc.attr, NewValue: "new"},
		}
		result, errs := ValidateOperations(ops, cli)
		require.Empty(t, errs, "attr=%s", tc.attr)
		assert.Equal(t, tc.expected, result[0].OldValue, "attr=%s", tc.attr)
	}
}

func TestValidateOperations_MixedValidity(t *testing.T) {
	cli := newMockADClient()
	cli.addEmployee(&employee.Employee{DN: "CN=Good,DC=x,DC=com"})

	ops := []MarkdownOperation{
		{DN: "CN=Good,DC=x,DC=com", Attribute: "mail", NewValue: "ok@x.com"},
		{DN: "CN=Missing,DC=x,DC=com", Attribute: "mail", NewValue: "x@x.com"},
	}
	result, errs := ValidateOperations(ops, cli)

	require.Len(t, errs, 1)
	assert.True(t, result[0].Valid)
	assert.False(t, result[1].Valid)
}

// ── ApplyOperations ───────────────────────────────────────────────────────────

func TestApplyOperations_Success(t *testing.T) {
	cli := newMockADClient()
	ops := []MarkdownOperation{
		{DN: "CN=A,DC=b,DC=com", Attribute: "telephoneNumber", NewValue: "+7 000", Valid: true},
	}
	results := ApplyOperations(ops, cli)

	require.Len(t, results, 1)
	assert.True(t, results[0].Success)
	assert.Empty(t, results[0].Error)
}

func TestApplyOperations_SkipsInvalidOps(t *testing.T) {
	cli := newMockADClient()
	ops := []MarkdownOperation{
		{DN: "CN=X,DC=y,DC=com", Attribute: "telephoneNumber", NewValue: "123",
			Valid: false, Error: "employee not found"},
	}
	results := ApplyOperations(ops, cli)

	require.Len(t, results, 1)
	assert.False(t, results[0].Success)
	assert.Equal(t, "employee not found", results[0].Error)
}

func TestApplyOperations_PartialSuccess(t *testing.T) {
	cli := newMockADClient()
	cli.setUpdateErr("CN=Fail,DC=x,DC=com", "mail", errors.New("LDAP error"))

	ops := []MarkdownOperation{
		{DN: "CN=Ok,DC=x,DC=com", Attribute: "mail", NewValue: "ok@x.com", Valid: true},
		{DN: "CN=Fail,DC=x,DC=com", Attribute: "mail", NewValue: "fail@x.com", Valid: true},
	}
	results := ApplyOperations(ops, cli)

	require.Len(t, results, 2)
	assert.True(t, results[0].Success)
	assert.False(t, results[1].Success)
	assert.Equal(t, "update failed", results[1].Error)
}

func TestApplyOperations_EmptyInput(t *testing.T) {
	cli := newMockADClient()
	results := ApplyOperations(nil, cli)
	assert.Empty(t, results)
}

// ── HTTP handler integration ──────────────────────────────────────────────────

func TestValidateMarkdownHandler_ValidDocument(t *testing.T) {
	cli := newMockADClient()
	cli.addEmployee(&employee.Employee{
		DN:              "CN=John Doe,OU=Employees,DC=company,DC=com",
		TelephoneNumber: "12345",
	})
	Init(cli)

	body := `{"markdown":"# Employee Data Corrections\n\n## Error 1: Wrong phone\n* **Employee:** ` +
		"`CN=John Doe,OU=Employees,DC=company,DC=com`" +
		`\n* **Attribute:** ` + "`telephoneNumber`" + `\n* **New value:** ` + "`+7 999 123 45 67`" + `\n"}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/markdown/validate",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ValidateMarkdownHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"valid":true`)
}

func TestValidateMarkdownHandler_InvalidJSON(t *testing.T) {
	Init(newMockADClient())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/markdown/validate",
		strings.NewReader("not-json"))
	w := httptest.NewRecorder()

	ValidateMarkdownHandler(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestValidateMarkdownHandler_MissingTitle(t *testing.T) {
	Init(newMockADClient())
	body := `{"markdown":"## Error 1: no title\n* **Employee:** CN=X\n* **Attribute:** mail\n* **New value:** x\n"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/markdown/validate",
		strings.NewReader(body))
	w := httptest.NewRecorder()

	ValidateMarkdownHandler(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestApplyMarkdownHandler_ReturnsAppliedCount(t *testing.T) {
	cli := newMockADClient()
	cli.addEmployee(&employee.Employee{
		DN:    "CN=Jane Smith,OU=Employees,DC=company,DC=com",
		Email: "old@x.com",
	})
	Init(cli)

	body := `{"markdown":"# Employee Data Corrections\n\n## Error 1: Fix email\n* **Employee:** ` +
		"`CN=Jane Smith,OU=Employees,DC=company,DC=com`" +
		`\n* **Attribute:** mail\n* **New value:** new@x.com\n"}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/markdown/apply",
		strings.NewReader(body))
	w := httptest.NewRecorder()

	ApplyMarkdownHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"applied":1`)
	assert.Contains(t, w.Body.String(), `"failed":0`)
}
