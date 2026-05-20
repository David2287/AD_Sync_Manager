package http_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httphandlers "ad-sync-manager/internal/handlers/http"
	"ad-sync-manager/internal/auth"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newLoginRouter sets up a minimal Gin router with the Login endpoint.
func newLoginRouter() *gin.Engine {
	r := gin.New()
	h := httphandlers.NewAuthHandler()
	r.POST("/login", h.Login)
	r.POST("/logout", h.Logout)
	r.GET("/me", h.Me)
	return r
}

// ── Login — request validation ────────────────────────────────────────────────

func TestLogin_MissingBody(t *testing.T) {
	r := newLoginRouter()
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "error")
}

func TestLogin_InvalidJSON(t *testing.T) {
	r := newLoginRouter()
	req := httptest.NewRequest(http.MethodPost, "/login",
		strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestLogin_MissingUsername(t *testing.T) {
	r := newLoginRouter()
	body := `{"password": "secret"}`
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "error")
}

func TestLogin_MissingPassword(t *testing.T) {
	r := newLoginRouter()
	body := `{"username": "jdoe"}`
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestLogin_EmptyBody(t *testing.T) {
	r := newLoginRouter()
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ── Logout ────────────────────────────────────────────────────────────────────

func TestLogout_NoToken_Accepted(t *testing.T) {
	r := newLoginRouter()
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	// Logout without a token is a no-op — still returns 204.
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestLogout_RevokesToken(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-that-is-at-least-32-chars!!")

	token, err := auth.GenerateJWT("alice", "CN=Alice,DC=company,DC=com", nil)
	require.NoError(t, err)

	// Token must be valid before logout.
	_, err = auth.ValidateJWT(token)
	require.NoError(t, err)

	r := newLoginRouter()
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	// Token must be rejected after logout.
	_, err = auth.ValidateJWT(token)
	assert.Error(t, err, "token must be revoked after logout")
}

// ── Me ────────────────────────────────────────────────────────────────────────

func TestMe_WithoutAuth_Unauthorized(t *testing.T) {
	r := newLoginRouter()
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	// No claims injected → handler returns 401.
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestMe_WithClaims_ReturnsProfile(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-that-is-at-least-32-chars!!")

	claims := &auth.Claims{Username: "bob", DN: "CN=Bob,DC=company,DC=com", Groups: []string{"CN=IT"}}

	r := gin.New()
	h := httphandlers.NewAuthHandler()
	r.GET("/me", func(c *gin.Context) {
		// Inject claims directly into the request context, mimicking RequireAuth.
		c.Request = c.Request.WithContext(auth.ContextWithClaims(c.Request.Context(), claims))
		h.Me(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "bob")
	assert.Contains(t, body, "CN=Bob,DC=company,DC=com")
}

// ── Integration: full auth flow (requires AD_URL) ────────────────────────────

func TestIntegration_Login_ValidCredentials(t *testing.T) {
	if os.Getenv("AD_URL") == "" {
		t.Skip("skipping integration test: AD_URL not set")
	}
	t.Setenv("JWT_SECRET", "integration-test-secret-32-chars-min!!")

	r := newLoginRouter()
	body := `{"username":"` + os.Getenv("TEST_USER") + `","password":"` + os.Getenv("TEST_PASSWORD") + `"}`
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "token")
}

func TestIntegration_Login_InvalidCredentials(t *testing.T) {
	if os.Getenv("AD_URL") == "" {
		t.Skip("skipping integration test: AD_URL not set")
	}
	t.Setenv("JWT_SECRET", "integration-test-secret-32-chars-min!!")

	r := newLoginRouter()
	body := `{"username":"nonexistent","password":"wrongpass"}`
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "invalid credentials")
}
