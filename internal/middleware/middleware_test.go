package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ad-sync-manager/internal/auth"
	"ad-sync-manager/internal/config"
	"ad-sync-manager/internal/middleware"
	"ad-sync-manager/pkg/logger"
)

func init() {
	gin.SetMode(gin.TestMode)
	// Suppress noisy Gin output during tests.
}

// jwtSecret used by all JWT tests.
const testSecret = "test-secret-that-is-at-least-32-chars!!"

func newBundle(adminGroupDN string) *middleware.Bundle {
	log, _ := logger.New("error", "json")
	return middleware.New(nil, log, adminGroupDN)
}

// makeToken creates a signed JWT with the given username, dn, and groups.
func makeToken(t *testing.T, username, dn string, groups []string) string {
	t.Helper()
	t.Setenv("JWT_SECRET", testSecret)
	auth.Init(config.ADConfig{}) // ensure auth package is initialised (no-op for JWT)
	tok, err := auth.GenerateJWT(username, dn, groups)
	require.NoError(t, err)
	return tok
}

// protectedRouter sets up a Gin router with RequireAuth on GET /protected.
func protectedRouter(b *middleware.Bundle) *gin.Engine {
	r := gin.New()
	secured := r.Group("/")
	secured.Use(b.RequireAuth())
	secured.GET("/protected", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return r
}

// ── RequireAuth ───────────────────────────────────────────────────────────────

func TestRequireAuth_MissingHeader_Returns401(t *testing.T) {
	t.Setenv("JWT_SECRET", testSecret)
	b := newBundle("")
	r := protectedRouter(b)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "error")
}

func TestRequireAuth_MalformedHeader_Returns401(t *testing.T) {
	t.Setenv("JWT_SECRET", testSecret)
	b := newBundle("")
	r := protectedRouter(b)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Token not-bearer-format")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireAuth_InvalidToken_Returns401(t *testing.T) {
	t.Setenv("JWT_SECRET", testSecret)
	b := newBundle("")
	r := protectedRouter(b)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer garbage.token.value")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireAuth_ValidToken_PassesThrough(t *testing.T) {
	t.Setenv("JWT_SECRET", testSecret)
	b := newBundle("")
	r := protectedRouter(b)

	token := makeToken(t, "jdoe", "CN=John,DC=company,DC=com", nil)
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireAuth_RevokedToken_Returns401(t *testing.T) {
	t.Setenv("JWT_SECRET", testSecret)
	b := newBundle("")
	r := protectedRouter(b)

	token := makeToken(t, "alice", "CN=Alice,DC=company,DC=com", nil)
	auth.RevokeToken(token)
	t.Cleanup(func() {
		// Revocation is in-process — no cleanup API, but the token expires anyway.
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ── RequireGroup ──────────────────────────────────────────────────────────────

// groupRouter returns a Gin router with RequireAuth + RequireGroup(groupDN) on GET /admin.
func groupRouter(b *middleware.Bundle, groupDN string) *gin.Engine {
	r := gin.New()
	secured := r.Group("/")
	secured.Use(b.RequireAuth())
	admin := secured.Group("/")
	admin.Use(b.RequireGroup(groupDN))
	admin.GET("/admin", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return r
}

func TestRequireGroup_UserInGroup_Allowed(t *testing.T) {
	t.Setenv("JWT_SECRET", testSecret)
	adminDN := "CN=IT-Admins,OU=Groups,DC=company,DC=com"
	b := newBundle(adminDN)
	r := groupRouter(b, adminDN)

	token := makeToken(t, "admin", "CN=Admin,DC=company,DC=com", []string{adminDN})
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireGroup_UserNotInGroup_Returns403(t *testing.T) {
	t.Setenv("JWT_SECRET", testSecret)
	adminDN := "CN=IT-Admins,OU=Groups,DC=company,DC=com"
	b := newBundle(adminDN)
	r := groupRouter(b, adminDN)

	// Token carries a different group — not in the admin group.
	token := makeToken(t, "user", "CN=User,DC=company,DC=com", []string{"CN=Developers,OU=Groups,DC=company,DC=com"})
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRequireGroup_UserNoGroups_Returns403(t *testing.T) {
	t.Setenv("JWT_SECRET", testSecret)
	adminDN := "CN=Admins,OU=Groups,DC=company,DC=com"
	b := newBundle(adminDN)
	r := groupRouter(b, adminDN)

	token := makeToken(t, "nogroups", "CN=NoGroups,DC=company,DC=com", nil)
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRequireGroup_CNOnlyMatch(t *testing.T) {
	t.Setenv("JWT_SECRET", testSecret)
	fullDN := "CN=IT-Admins,OU=Groups,DC=company,DC=com"
	b := newBundle("")
	r := groupRouter(b, "IT-Admins") // CN-only match

	token := makeToken(t, "admin", "CN=Admin,DC=company,DC=com", []string{fullDN})
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireGroup_EmptyGroupDN_AllPass(t *testing.T) {
	t.Setenv("JWT_SECRET", testSecret)
	b := newBundle("")
	r := groupRouter(b, "") // no restriction

	token := makeToken(t, "anyone", "CN=Anyone,DC=company,DC=com", nil)
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
