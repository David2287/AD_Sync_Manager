package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"ad-sync-manager/internal/auth"
)

const ginClaimsKey = "auth_claims"

// RequireAuth is the Gin middleware that validates Bearer JWTs.
//
// It delegates all token logic to auth.ValidateJWT (signature, expiry, deny-list)
// and stores the *auth.Claims in the Gin context under ginClaimsKey so that
// downstream handlers can call GinClaimsFrom(c).
//
// Requests without a valid token receive 401 Unauthorized and the chain halts.
func (b *Bundle) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		rawToken := extractBearer(c.GetHeader("Authorization"))
		if rawToken == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing or malformed Authorization header",
			})
			return
		}

		claims, err := auth.ValidateJWT(rawToken)
		if err != nil {
			b.log.Warn("rejected invalid token", "ip", c.ClientIP(), "error", err)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid or expired token",
			})
			return
		}

		// Store claims in both the Gin context (for Gin handlers) and the
		// standard request context (for net/http handlers wrapped via gin.WrapF).
		c.Set(ginClaimsKey, claims)
		ctx := auth.ContextWithClaims(c.Request.Context(), claims)

		// When user-bind mode is active, look up the cached LDAP credentials for
		// this token and inject them so that repository functions can bind as the
		// authenticated user instead of the service account.
		if b.useUserBind {
			if cred, ok := auth.LookupCredential(rawToken); ok {
				ctx = auth.ContextWithLDAPCred(ctx, cred)
			}
		}

		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

// GinClaimsFrom retrieves the *auth.Claims stored by RequireAuth.
// Panics if called on a route that does not use RequireAuth — intentional,
// as it signals a routing configuration bug.
func GinClaimsFrom(c *gin.Context) *auth.Claims {
	return c.MustGet(ginClaimsKey).(*auth.Claims)
}

func extractBearer(header string) string {
	const prefix = "Bearer "
	if len(header) < len(prefix) || header[:len(prefix)] != prefix {
		return ""
	}
	return header[len(prefix):]
}

// Session is a lightweight view of the authenticated user derived from their
// JWT claims. It exists so that Gin handlers written before Stage 2 (e.g. the
// note handlers) can call SessionFrom(c) without directly importing the auth
// package or knowing about JWT internals.
type Session struct {
	UserID string // sAMAccountName from the JWT subject
}

// SessionFrom extracts the current user's Session from the Gin context.
// Must be called on routes protected by RequireAuth; panics otherwise
// (same contract as GinClaimsFrom).
func SessionFrom(c *gin.Context) Session {
	return Session{UserID: GinClaimsFrom(c).Username}
}

// GinWrapNetHTTP adapts a standard net/http middleware (like auth.AuthMiddleware)
// into a Gin middleware. Useful when you want to reuse framework-agnostic
// middleware directly in the Gin chain without duplicating logic.
//
// Example:
//
//	r.Use(GinWrapNetHTTP(auth.AuthMiddleware))
func GinWrapNetHTTP(mw func(http.Handler) http.Handler) gin.HandlerFunc {
	return func(c *gin.Context) {
		var called bool
		mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c.Request = r // pick up any context values the middleware added
			called = true
			c.Next()
		})).ServeHTTP(c.Writer, c.Request)

		if !called {
			c.Abort() // middleware rejected the request (wrote 401/403)
		}
	}
}
