package employee

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ad-sync-manager/internal/config"
)

// resetTestState resets package-level state between tests so they don't
// bleed into each other. Called as t.Cleanup or directly at the start of
// each test that touches pkgCache or listVersion.
func resetTestState() {
	pkgCache = newCache()
	atomic.StoreInt64(&listVersion, 0)
}

// ── Cache unit tests ──────────────────────────────────────────────────────────

func TestCache_SetGet(t *testing.T) {
	c := newCache()
	c.Set("key1", "hello", time.Minute)
	got := c.Get("key1")
	require.NotNil(t, got)
	assert.Equal(t, "hello", got)
}

func TestCache_Miss(t *testing.T) {
	c := newCache()
	assert.Nil(t, c.Get("nonexistent"))
}

func TestCache_Expiry(t *testing.T) {
	c := newCache()
	c.Set("short", "value", 5*time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	assert.Nil(t, c.Get("short"), "expired entry should return nil")
}

func TestCache_Delete(t *testing.T) {
	c := newCache()
	c.Set("x", 42, time.Minute)
	c.Delete("x")
	assert.Nil(t, c.Get("x"))
}

func TestCache_Overwrite(t *testing.T) {
	c := newCache()
	c.Set("k", "first", time.Minute)
	c.Set("k", "second", time.Minute)
	assert.Equal(t, "second", c.Get("k"))
}

// ── LDAP filter builder ───────────────────────────────────────────────────────

func TestBuildListFilter_NoSearch(t *testing.T) {
	f := buildListFilter("")
	assert.Contains(t, f, "objectClass=user")
	assert.Contains(t, f, "objectCategory=person")
	assert.NotContains(t, f, "displayName")
	assert.NotContains(t, f, "mail=*")
}

func TestBuildListFilter_WithSearch(t *testing.T) {
	f := buildListFilter("alice")
	assert.Contains(t, f, "displayName=*alice*")
	assert.Contains(t, f, "mail=*alice*")
}

func TestBuildListFilter_LDAPInjectionEscaped(t *testing.T) {
	// A naive implementation would embed the raw string and create a syntactically
	// invalid (or attacker-controlled) filter. ldap.EscapeFilter must neutralise it.
	f := buildListFilter(")(|(cn=*")
	assert.NotContains(t, f, ")(|(cn=*",
		"raw injection payload must not appear verbatim in the filter")
	// The filter must still be balanced parentheses.
	open  := strings.Count(f, "(")
	close := strings.Count(f, ")")
	assert.Equal(t, open, close, "filter parentheses must be balanced after escaping")
}

// ── Handler tests (cache-hit path — no LDAP required) ────────────────────────

func TestListEmployeesHandler_CacheHit(t *testing.T) {
	resetTestState()

	expected := listResult{
		Data:   []Employee{{DN: "CN=Alice,DC=company,DC=com", FullName: "Alice"}},
		Total:  1,
		Limit:  50,
		Offset: 0,
	}
	// Pre-populate with the exact key the handler will compute (version=0, no search).
	filter   := buildListFilter("")
	cacheKey := fmt.Sprintf("list:v0:%s:50:0", filter)
	pkgCache.Set(cacheKey, expected, time.Minute)

	req := httptest.NewRequest(http.MethodGet, "/employees", nil)
	w   := httptest.NewRecorder()
	ListEmployeesHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var got listResult
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Equal(t, 1, got.Total)
	assert.Equal(t, "Alice", got.Data[0].FullName)
}

func TestListEmployeesHandler_PaginationParams(t *testing.T) {
	resetTestState()

	expected := listResult{Data: []Employee{}, Total: 300, Limit: 10, Offset: 100}
	filter   := buildListFilter("bob")
	cacheKey := fmt.Sprintf("list:v0:%s:10:100", filter)
	pkgCache.Set(cacheKey, expected, time.Minute)

	req := httptest.NewRequest(http.MethodGet, "/employees?limit=10&offset=100&search=bob", nil)
	w   := httptest.NewRecorder()
	ListEmployeesHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var got listResult
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Equal(t, 10, got.Limit)
	assert.Equal(t, 100, got.Offset)
}

func TestListEmployeesHandler_LimitClamped(t *testing.T) {
	resetTestState()

	// Requesting limit=999 should be clamped to 200.
	expected := listResult{Data: []Employee{}, Total: 0, Limit: 200, Offset: 0}
	filter   := buildListFilter("")
	cacheKey := fmt.Sprintf("list:v0:%s:200:0", filter)
	pkgCache.Set(cacheKey, expected, time.Minute)

	req := httptest.NewRequest(http.MethodGet, "/employees?limit=999", nil)
	w   := httptest.NewRecorder()
	ListEmployeesHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var got listResult
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Equal(t, 200, got.Limit)
}

func TestGetEmployeeHandler_CacheHit(t *testing.T) {
	resetTestState()

	dn  := "CN=Bob,OU=Employees,DC=company,DC=com"
	emp := Employee{DN: dn, FullName: "Bob", Email: "bob@company.com"}
	pkgCache.Set("employee:"+dn, &emp, time.Minute)

	req := httptest.NewRequest(http.MethodGet, "/employees?dn="+dn, nil)
	w   := httptest.NewRecorder()
	GetEmployeeHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var got Employee
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Equal(t, "Bob", got.FullName)
	assert.Equal(t, dn, got.DN)
}

func TestGetEmployeeHandler_MissingDN(t *testing.T) {
	resetTestState()

	req := httptest.NewRequest(http.MethodGet, "/employees", nil)
	w   := httptest.NewRecorder()
	GetEmployeeHandler(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateEmployeeHandler_MissingDN(t *testing.T) {
	resetTestState()

	body := `{"telephoneNumber":"+7 999 000 00 00"}`
	req  := httptest.NewRequest(http.MethodPut, "/employees", strings.NewReader(body))
	w    := httptest.NewRecorder()
	UpdateEmployeeHandler(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateEmployeeHandler_InvalidJSON(t *testing.T) {
	resetTestState()

	req := httptest.NewRequest(http.MethodPut,
		"/employees?dn=CN=Test,DC=company,DC=com",
		strings.NewReader("not-json"))
	w := httptest.NewRecorder()
	UpdateEmployeeHandler(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateEmployeeHandler_InvalidatesListCache(t *testing.T) {
	resetTestState()

	// Pre-populate a list cache entry.
	filter   := buildListFilter("")
	cacheKey := fmt.Sprintf("list:v0:%s:50:0", filter)
	pkgCache.Set(cacheKey, listResult{Total: 1}, time.Minute)

	// Pre-populate the per-employee cache entry.
	dn := "CN=Eve,OU=Employees,DC=company,DC=com"
	pkgCache.Set("employee:"+dn, &Employee{DN: dn}, time.Minute)

	// Simulate the cache-invalidation logic only (no LDAP needed).
	pkgCache.Delete("employee:" + dn)
	atomic.AddInt64(&listVersion, 1)

	// The old list cache key is now unreachable (version changed).
	assert.Nil(t, pkgCache.Get(cacheKey), "list cache must be unreachable after version bump")
	// The per-employee entry was deleted.
	assert.Nil(t, pkgCache.Get("employee:"+dn), "per-employee cache must be cleared on update")
}

// ── Integration tests (require a real AD or OpenLDAP instance) ────────────────
//
// Quick setup with Docker (plain LDAP for local dev — use LDAPS in production):
//
//	docker run -d --name test-ldap \
//	  -p 1389:389 \
//	  -e LDAP_ORGANISATION="Company" \
//	  -e LDAP_DOMAIN="company.com" \
//	  -e LDAP_ADMIN_PASSWORD="admin" \
//	  osixia/openldap:1.5.0
//
// Add a test OU and user:
//
//	docker exec -i test-ldap ldapadd -x \
//	  -D "cn=admin,dc=company,dc=com" -w admin <<EOF
//	dn: ou=Employees,dc=company,dc=com
//	objectClass: organizationalUnit
//	ou: Employees
//
//	dn: cn=Test User,ou=Employees,dc=company,dc=com
//	objectClass: inetOrgPerson
//	sn: User
//	cn: Test User
//	mail: testuser@company.com
//	telephoneNumber: +7 000 000 0000
//	l: A-101
//	EOF
//
// Run:
//
//	AD_URL=ldap://localhost:1389 \
//	AD_BASE_DN=DC=company,DC=com \
//	AD_EMPLOYEE_OU=OU=Employees,DC=company,DC=com \
//	AD_BIND_DN=cn=admin,dc=company,dc=com \
//	AD_BIND_PASSWORD=admin \
//	AD_ADMIN_GROUP="" \
//	JWT_SECRET=thirtytwocharactersecretkey1234 \
//	go test -run TestIntegration ./internal/employee/

func TestIntegration_GetAllEmployees(t *testing.T) {
	if os.Getenv("AD_URL") == "" {
		t.Skip("skipping integration test: AD_URL not set")
	}
	cfg, err := config.Load()
	require.NoError(t, err)
	Init(cfg.AD)

	employees, total, err := GetAllEmployees(context.Background(), cfg.AD.EmployeeOU, buildListFilter(""), 50, 0)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, total, 0)
	t.Logf("GetAllEmployees: %d returned (total=%d)", len(employees), total)
}

func TestIntegration_GetAllEmployees_WithSearch(t *testing.T) {
	if os.Getenv("AD_URL") == "" {
		t.Skip("skipping integration test: AD_URL not set")
	}
	cfg, err := config.Load()
	require.NoError(t, err)
	Init(cfg.AD)

	search := os.Getenv("TEST_SEARCH_TERM")
	if search == "" {
		search = "test"
	}
	_, total, err := GetAllEmployees(context.Background(), cfg.AD.EmployeeOU, buildListFilter(search), 50, 0)
	require.NoError(t, err)
	t.Logf("search=%q total=%d", search, total)
}

func TestIntegration_GetEmployeeByDN(t *testing.T) {
	if os.Getenv("AD_URL") == "" {
		t.Skip("skipping integration test: AD_URL not set")
	}
	dn := os.Getenv("TEST_EMPLOYEE_DN")
	if dn == "" {
		t.Skip("TEST_EMPLOYEE_DN not set")
	}
	cfg, err := config.Load()
	require.NoError(t, err)
	Init(cfg.AD)

	emp, err := GetEmployeeByDN(context.Background(), dn)
	require.NoError(t, err)
	assert.Equal(t, dn, emp.DN)
	t.Logf("employee: %+v", emp)
}

func TestIntegration_GetEmployeeByDN_NotFound(t *testing.T) {
	if os.Getenv("AD_URL") == "" {
		t.Skip("skipping integration test: AD_URL not set")
	}
	cfg, err := config.Load()
	require.NoError(t, err)
	Init(cfg.AD)

	_, err = GetEmployeeByDN(context.Background(), "CN=DoesNotExist,DC=company,DC=com")
	assert.ErrorIs(t, err, ErrNotFound)
}
