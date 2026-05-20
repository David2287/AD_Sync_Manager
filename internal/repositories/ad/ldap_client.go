package ad

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"

	"ad-sync-manager/internal/config"
	"ad-sync-manager/internal/domain/entity"
	"ad-sync-manager/internal/domain/interfaces"
)

// ldapClient implements interfaces.ADClient against a real LDAP/AD server.
type ldapClient struct {
	cfg  config.ADConfig
	conn *ldap.Conn
}

// NewLDAPClient dials the AD server using the service-account bind DN and
// returns a ready-to-use ADClient. The connection is kept alive for the
// lifetime of the process; reconnection logic can be added in Stage 2.
func NewLDAPClient(cfg config.ADConfig) (interfaces.ADClient, error) {
	conn, err := dialTLS(cfg)
	if err != nil {
		return nil, err
	}

	// Bind with the service account so we can run search queries.
	if err := conn.Bind(cfg.BindDN, cfg.BindPassword); err != nil {
		conn.Close()
		return nil, fmt.Errorf("ldap: service account bind failed: %w", err)
	}

	return &ldapClient{cfg: cfg, conn: conn}, nil
}

// Authenticate performs a per-user bind to verify credentials, then fetches
// the user's group memberships using the service account connection.
func (c *ldapClient) Authenticate(ctx context.Context, username, password string) ([]string, error) {
	// Find the user's full DN first (needed for bind).
	userDN, groups, err := c.searchUser(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("ldap: user lookup: %w", err)
	}

	// Open a second connection just for the user bind so we do not disturb
	// the service-account connection.
	userConn, err := dialTLS(c.cfg)
	if err != nil {
		return nil, err
	}
	defer userConn.Close()

	if err := userConn.Bind(userDN, password); err != nil {
		return nil, fmt.Errorf("ldap: credential check failed")
	}

	return groups, nil
}

// SyncEmployees fetches all accounts from the EmployeeOU.
func (c *ldapClient) SyncEmployees(ctx context.Context) ([]*entity.Employee, error) {
	filter := "(&(objectClass=person)(objectCategory=person)(!(userAccountControl:1.2.840.113556.1.4.803:=2)))"

	req := ldap.NewSearchRequest(
		c.cfg.EmployeeOU,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0, 0, false,
		filter,
		c.cfg.Attributes,
		nil,
	)

	res, err := c.conn.SearchWithPaging(req, 500)
	if err != nil {
		return nil, fmt.Errorf("ldap: employee search: %w", err)
	}

	employees := make([]*entity.Employee, 0, len(res.Entries))
	for _, e := range res.Entries {
		employees = append(employees, entryToEmployee(e))
	}
	return employees, nil
}

// GetEmployee retrieves a single employee by sAMAccountName.
func (c *ldapClient) GetEmployee(_ context.Context, username string) (*entity.Employee, error) {
	_, _, err := c.searchUser(context.Background(), username)
	if err != nil {
		return nil, err
	}

	filter := fmt.Sprintf("(&(objectClass=person)(sAMAccountName=%s))",
		ldap.EscapeFilter(username))

	req := ldap.NewSearchRequest(
		c.cfg.BaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		1, 0, false,
		filter,
		c.cfg.Attributes,
		nil,
	)

	res, err := c.conn.Search(req)
	if err != nil {
		return nil, fmt.Errorf("ldap: get employee %q: %w", username, err)
	}
	if len(res.Entries) == 0 {
		return nil, fmt.Errorf("ldap: employee %q not found", username)
	}
	return entryToEmployee(res.Entries[0]), nil
}

func (c *ldapClient) Close() error {
	c.conn.Close()
	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func dialTLS(cfg config.ADConfig) (*ldap.Conn, error) {
	tlsCfg := &tls.Config{
		InsecureSkipVerify: cfg.TLSSkipVerify, //nolint:gosec
		ServerName:         serverName(cfg.URL),
	}
	conn, err := ldap.DialURL(cfg.URL, ldap.DialWithTLSConfig(tlsCfg))
	if err != nil {
		return nil, fmt.Errorf("ldap: dial %q: %w", cfg.URL, err)
	}
	conn.SetTimeout(10 * time.Second)
	return conn, nil
}

// searchUser returns the DN and memberOf list for a given sAMAccountName.
func (c *ldapClient) searchUser(_ context.Context, username string) (dn string, groups []string, err error) {
	filter := fmt.Sprintf("(&(objectClass=person)(sAMAccountName=%s))",
		ldap.EscapeFilter(username))

	req := ldap.NewSearchRequest(
		c.cfg.BaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		1, 0, false,
		filter,
		[]string{"dn", "memberOf"},
		nil,
	)

	res, err := c.conn.Search(req)
	if err != nil {
		return "", nil, fmt.Errorf("ldap: user search: %w", err)
	}
	if len(res.Entries) == 0 {
		return "", nil, fmt.Errorf("ldap: user %q not found", username)
	}

	entry := res.Entries[0]
	return entry.DN, entry.GetAttributeValues("memberOf"), nil
}

func entryToEmployee(e *ldap.Entry) *entity.Employee {
	return &entity.Employee{
		ID:          e.GetAttributeValue("sAMAccountName"),
		DisplayName: e.GetAttributeValue("displayName"),
		Mail:        e.GetAttributeValue("mail"),
		Phone:       e.GetAttributeValue("telephoneNumber"),
		Office:      e.GetAttributeValue("physicalDeliveryOfficeName"),
		Groups:      e.GetAttributeValues("memberOf"),
		SyncedAt:    time.Now(),
	}
}

// serverName extracts the hostname from an ldaps:// URL for TLS SNI.
func serverName(url string) string {
	host := strings.TrimPrefix(url, "ldaps://")
	host = strings.TrimPrefix(host, "ldap://")
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}
	return host
}
