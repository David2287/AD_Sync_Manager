// Package http contains Gin HTTP handlers.
// The core auth logic lives in internal/auth; these handlers are thin Gin
// adapters that delegate to it and format responses as JSON.
package http

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"ad-sync-manager/internal/audit"
	"ad-sync-manager/internal/auth"
)

// AuthHandler groups all authentication endpoints.
type AuthHandler struct {
	useUserBind bool
}

// NewAuthHandler constructs an AuthHandler.
// Pass useUserBind=true when AD_USE_USER_BIND is enabled so that the handler
// stores per-user LDAP credentials in the in-memory cache on login and clears
// them on logout.
func NewAuthHandler(useUserBind bool) *AuthHandler {
	return &AuthHandler{useUserBind: useUserBind}
}

// Login handles POST /api/v1/auth/login and POST /api/login.
//
//	Request body:  {"username": "jdoe", "password": "secret"}
//	Success (200): {"token": "...", "username": "jdoe", "dn": "CN=..."}
//	Failure (401): {"error": "invalid credentials"}
//
// Passwords are never logged. Only the outcome (ok/fail) is surfaced.
func (h *AuthHandler) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username and password are required"})
		return
	}

	ok, dn, groups, err := auth.AuthenticateAD(req.Username, req.Password)
	if err != nil {
		_ = c.Error(err)
		audit.LogLogin(req.Username, c.ClientIP(), false, "authentication service unavailable")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "authentication service unavailable"})
		return
	}
	if !ok {
		audit.LogLogin(req.Username, c.ClientIP(), false, "invalid credentials")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	token, err := auth.GenerateJWT(req.Username, dn, groups)
	if err != nil {
		_ = c.Error(err)
		audit.LogLogin(req.Username, c.ClientIP(), false, "token generation failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not issue token"})
		return
	}

	claims, _ := auth.ValidateJWT(token) // safe: we just generated it

	// When user-bind mode is active, cache the user's credentials for the JWT
	// lifetime so that subsequent LDAP operations can bind as this user.
	// The password lives only in process memory and is cleared on logout.
	if h.useUserBind {
		auth.StoreCredential(token, dn, req.Password, claims.ExpiresAt.Time)
	}

	audit.LogLogin(req.Username, c.ClientIP(), true, "")

	c.JSON(http.StatusOK, gin.H{
		"token":      token,
		"username":   req.Username,
		"dn":         dn,
		"expires_at": claims.ExpiresAt.Time,
	})
}

// Logout handles POST /api/v1/auth/logout.
// Adds the current token to the deny-list so ValidateJWT will reject it.
// When user-bind mode is active it also removes the cached credentials.
func (h *AuthHandler) Logout(c *gin.Context) {
	raw := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	raw = strings.TrimSpace(raw)
	if raw != "" {
		auth.RevokeToken(raw)
		if h.useUserBind {
			auth.DeleteCredential(raw)
		}
	}
	c.Status(http.StatusNoContent)
}

// Me handles GET /api/v1/me and GET /api/me.
// Returns the authenticated user's profile from the JWT claims.
// Must be served behind GinAuthMiddleware.
func (h *AuthHandler) Me(c *gin.Context) {
	claims := auth.ClaimsFromContext(c.Request.Context())
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"username": claims.Username,
		"dn":       claims.DN,
		"groups":   claims.Groups,
	})
}
