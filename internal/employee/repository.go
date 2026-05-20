package employee

import (
	"crypto/tls"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"

	"ad-sync-manager/internal/config"
)

// ErrNotFound is returned when the requested employee DN does not exist in AD.
var ErrNotFound = errors.New("employee not found")

// adCfg holds the AD configuration stored by Init.
var adCfg config.ADConfig

// Init stores the AD configuration used by all repository and handler functions
// in this package. Must be called once at startup, before any HTTP request
// reaches these handlers.
func Init(cfg config.ADConfig) {
	adCfg = cfg
}

// GetAllEmployees returns a paginated slice of employees matching filter and the
// total count of all matching records (without pagination).
//
// baseDN scopes the search (e.g. config.ADConfig.EmployeeOU).
// filter must be a complete, already-sanitised LDAP filter string.
// limit and offset control which slice of results is returned; the total always
// reflects the full match count.
func GetAllEmployees(baseDN, filter string, limit, offset int) ([]Employee, int, error) {
	conn, err := dialBind()
	if err != nil {
		return nil, 0, err
	}
	defer conn.Close()

	attrs := []string{"displayName", "mail", "physicalDeliveryOfficeName", "telephoneNumber"}
	req := ldap.NewSearchRequest(
		baseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0, 0, false,
		filter,
		attrs,
		nil,
	)
	res, err := conn.SearchWithPaging(req, 500)
	if err != nil {
		return nil, 0, fmt.Errorf("employee: list search: %w", err)
	}

	all := make([]Employee, 0, len(res.Entries))
	for _, e := range res.Entries {
		all = append(all, entryToEmployee(e))
	}

	total := len(all)
	if offset >= total {
		return []Employee{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return all[offset:end], total, nil
}

// GetEmployeeByDN fetches a single employee by their full Distinguished Name.
// Returns ErrNotFound when the DN does not exist in the directory.
func GetEmployeeByDN(dn string) (*Employee, error) {
	conn, err := dialBind()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	attrs := []string{"displayName", "mail", "physicalDeliveryOfficeName", "telephoneNumber"}
	req := ldap.NewSearchRequest(
		dn,
		ldap.ScopeBaseObject, // direct lookup — the DN is both the base and the target
		ldap.NeverDerefAliases,
		1, 0, false,
		"(objectClass=*)",
		attrs,
		nil,
	)
	res, err := conn.Search(req)
	if err != nil {
		var ldapErr *ldap.Error
		if errors.As(err, &ldapErr) && ldapErr.ResultCode == ldap.LDAPResultNoSuchObject {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("employee: get by DN %q: %w", dn, err)
	}
	if len(res.Entries) == 0 {
		return nil, ErrNotFound
	}

	emp := entryToEmployee(res.Entries[0])
	emp.DN = dn
	return &emp, nil
}

// UpdateEmployeeAttribute replaces a single LDAP attribute on the entry
// identified by dn. Only attributes in the editableAttrs allowlist may be
// modified; others return an error without touching AD.
func UpdateEmployeeAttribute(dn, attrName, newValue string) error {
	if !editableAttrs[attrName] {
		return fmt.Errorf("employee: attribute %q is not editable", attrName)
	}
	conn, err := dialBind()
	if err != nil {
		return err
	}
	defer conn.Close()

	mod := ldap.NewModifyRequest(dn, nil)
	mod.Replace(attrName, []string{newValue})
	if err := conn.Modify(mod); err != nil {
		return fmt.Errorf("employee: modify %q → %s: %w", dn, attrName, err)
	}
	return nil
}

// ── LDAP filter builder ───────────────────────────────────────────────────────

// editableAttrs is the allowlist of LDAP attribute names that may be changed via
// the employee REST API or the markdown correction workflow.
var editableAttrs = map[string]bool{
	"telephoneNumber":            true,
	"physicalDeliveryOfficeName": true,
	"mail":                       true,
	"displayName":                true,
}

// buildListFilter constructs a safe LDAP filter for the list endpoint.
// When search is non-empty it is escaped with ldap.EscapeFilter before being
// interpolated into the displayName and mail wildcard clauses.
func buildListFilter(search string) string {
	// Exclude disabled accounts (bit 2 of userAccountControl).
	base := "(&(objectClass=user)(objectCategory=person)(!(userAccountControl:1.2.840.113556.1.4.803:=2))"
	if search == "" {
		return base + ")"
	}
	s := ldap.EscapeFilter(search)
	return fmt.Sprintf("%s(|(displayName=*%s*)(mail=*%s*)))", base, s, s)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func dialBind() (*ldap.Conn, error) {
	tlsCfg := &tls.Config{
		InsecureSkipVerify: adCfg.TLSSkipVerify, //nolint:gosec
		ServerName:         serverHostname(adCfg.URL),
	}
	conn, err := ldap.DialURL(adCfg.URL, ldap.DialWithTLSConfig(tlsCfg))
	if err != nil {
		return nil, fmt.Errorf("employee: LDAP dial: %w", err)
	}
	conn.SetTimeout(10 * time.Second)
	if err := conn.Bind(adCfg.BindDN, adCfg.BindPassword); err != nil {
		conn.Close()
		return nil, fmt.Errorf("employee: service-account bind: %w", err)
	}
	return conn, nil
}

func entryToEmployee(e *ldap.Entry) Employee {
	return Employee{
		DN:              e.DN,
		FullName:        e.GetAttributeValue("displayName"),
		Email:           e.GetAttributeValue("mail"),
		Office:          e.GetAttributeValue("physicalDeliveryOfficeName"),
		TelephoneNumber: e.GetAttributeValue("telephoneNumber"),
	}
}

// serverHostname extracts the hostname from an ldaps:// or ldap:// URL for TLS SNI.
func serverHostname(rawURL string) string {
	host := strings.TrimPrefix(rawURL, "ldaps://")
	host = strings.TrimPrefix(host, "ldap://")
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}
	return host
}
