// Package auth provides Active Directory authentication, JWT management,
// and HTTP middleware for the AD-Sync Manager application.
//
// Call Init() once at startup before using any other function in this package.
package auth

import (
	"crypto/tls"
	"fmt"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"

	"ad-sync-manager/internal/config"
)

// adCfg is the package-level AD configuration, set once via Init().
var adCfg config.ADConfig

// Init must be called once at application startup before AuthenticateAD or
// GetUserGroups are used. It is safe to call multiple times (last write wins),
// but there is no concurrency protection — call before spawning goroutines.
func Init(cfg config.ADConfig) {
	adCfg = cfg
}

// AuthenticateAD validates user credentials against Active Directory.
//
// Return contract:
//   - (true,  dn, groups, nil)  — credentials valid
//   - (false, "",    nil, nil)  — wrong username or password (not an error)
//   - (false, "",    nil, err)  — infrastructure failure (LDAP down, bind error)
//
// Passwords are never logged. Only the success/failure outcome is surfaced.
// Empty passwords are rejected immediately; AD may accept them on misconfigured DCs.
func AuthenticateAD(username, password string) (ok bool, dn string, groups []string, err error) {
	if strings.TrimSpace(password) == "" {
		return false, "", nil, nil
	}

	// ── Step 1: service-account bind to resolve the user's DN ──────────────
	svcConn, err := dialLDAPS()
	if err != nil {
		return false, "", nil, fmt.Errorf("auth: service dial: %w", err)
	}
	defer svcConn.Close()

	if err := svcConn.Bind(adCfg.BindDN, adCfg.BindPassword); err != nil {
		return false, "", nil, fmt.Errorf("auth: service bind: %w", err)
	}

	userDN, err := findUserDN(svcConn, username)
	if err != nil {
		// User not found — treat as wrong credentials rather than exposing
		// account existence to the caller.
		return false, "", nil, nil
	}

	// ── Step 2: user bind on a separate connection to verify the password ──
	userConn, err := dialLDAPS()
	if err != nil {
		return false, "", nil, fmt.Errorf("auth: user dial: %w", err)
	}
	defer userConn.Close()

	if err := userConn.Bind(userDN, password); err != nil {
		return false, "", nil, nil // wrong password — not an infrastructure error
	}

	// ── Step 3: resolve group memberships via the service connection ────────
	// We deliberately use svcConn here: the user's own bind may lack read
	// permissions on other group objects in restricted AD configurations.
	groups, err = GetUserGroups(svcConn, userDN)
	if err != nil {
		return false, "", nil, fmt.Errorf("auth: group fetch: %w", err)
	}

	return true, userDN, groups, nil
}

// GetUserGroups returns all group DNs the user belongs to, including nested
// (transitive) memberships.
//
// It uses the AD-specific LDAP_MATCHING_RULE_IN_CHAIN OID
// (1.2.840.113556.1.4.1941), which resolves the full transitive group chain
// on the server in a single query — no client-side recursion needed.
//
// Falls back to reading the memberOf attribute directly if the OID is
// unavailable (e.g. the service account lacks permission to use it, or the
// server is OpenLDAP rather than AD). The fallback gives only direct memberships.
func GetUserGroups(conn *ldap.Conn, userDN string) ([]string, error) {
	filter := fmt.Sprintf(
		"(&(objectClass=group)(member:1.2.840.113556.1.4.1941:=%s))",
		ldap.EscapeFilter(userDN),
	)

	req := ldap.NewSearchRequest(
		adCfg.BaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0,  // no size limit (paging handles it)
		30, // time limit seconds
		false,
		filter,
		[]string{"dn"},
		nil,
	)

	res, err := conn.SearchWithPaging(req, 200)
	if err != nil {
		// OID may be restricted on this DC — fall back to direct memberOf.
		return memberOfFallback(conn, userDN)
	}

	groups := make([]string, 0, len(res.Entries))
	for _, e := range res.Entries {
		groups = append(groups, e.DN)
	}
	return groups, nil
}

// memberOfFallback reads the memberOf attribute directly from the user object.
// Returns direct group memberships only (no transitive resolution).
func memberOfFallback(conn *ldap.Conn, userDN string) ([]string, error) {
	req := ldap.NewSearchRequest(
		userDN,
		ldap.ScopeBaseObject,
		ldap.NeverDerefAliases,
		1, 10, false,
		"(objectClass=*)",
		[]string{"memberOf"},
		nil,
	)

	res, err := conn.Search(req)
	if err != nil {
		return nil, fmt.Errorf("auth: memberOf fallback: %w", err)
	}
	if len(res.Entries) == 0 {
		return nil, nil
	}
	return res.Entries[0].GetAttributeValues("memberOf"), nil
}

// findUserDN resolves a sAMAccountName to its full Distinguished Name.
// LDAP attribute matching in AD is case-insensitive by default.
func findUserDN(conn *ldap.Conn, username string) (string, error) {
	filter := fmt.Sprintf(
		"(&(objectClass=person)(sAMAccountName=%s))",
		ldap.EscapeFilter(username),
	)

	req := ldap.NewSearchRequest(
		adCfg.BaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		1, 10, false,
		filter,
		[]string{"dn"},
		nil,
	)

	res, err := conn.Search(req)
	if err != nil {
		return "", fmt.Errorf("auth: DN search: %w", err)
	}
	if len(res.Entries) == 0 {
		return "", fmt.Errorf("auth: user %q not found in AD", username)
	}
	return res.Entries[0].DN, nil
}

// dialLDAPS opens a new TLS-encrypted connection to the configured AD endpoint.
// Each call returns a fresh connection; callers are responsible for closing it.
func dialLDAPS() (*ldap.Conn, error) {
	tlsCfg := &tls.Config{
		InsecureSkipVerify: adCfg.TLSSkipVerify, //nolint:gosec — flag is false in production
		ServerName:         serverHostname(adCfg.URL),
	}

	conn, err := ldap.DialURL(adCfg.URL, ldap.DialWithTLSConfig(tlsCfg))
	if err != nil {
		return nil, fmt.Errorf("auth: ldap dial %q: %w", adCfg.URL, err)
	}

	conn.SetTimeout(10 * time.Second)
	return conn, nil
}

// serverHostname strips scheme and port from an ldaps:// URL for TLS SNI.
func serverHostname(rawURL string) string {
	host := strings.TrimPrefix(rawURL, "ldaps://")
	host = strings.TrimPrefix(host, "ldap://")
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}
	return host
}
