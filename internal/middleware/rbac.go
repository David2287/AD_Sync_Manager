package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"ad-sync-manager/internal/auth"
)

// RequireAdmin allows the request only when the session includes the configured
// AD admin group DN (from config.ADConfig.AdminGroupDN).
//
// Must be chained after RequireAuth. Returns 403 if the user is not an admin.
// If no admin group is configured (empty string), all authenticated users pass.
func (b *Bundle) RequireAdmin() gin.HandlerFunc {
	return b.RequireGroup(b.adminGroupDN)
}

// RequireGroup builds a Gin middleware that enforces membership in a specific
// AD group. groupName may be a full DN or a CN-only name (case-insensitive).
//
// Delegates matching logic to auth.RequireGroupMiddleware, keeping the rule
// consistent between the Gin and plain net/http stacks.
//
// Must be chained after RequireAuth.
//
// Example:
//
//	admin := secured.Group("/")
//	admin.Use(mw.RequireGroup("AD-Sync-Admins"))
//	admin.POST("/sync", syncHandler.Run)
func (b *Bundle) RequireGroup(groupName string) gin.HandlerFunc {
	if groupName == "" {
		// No restriction — pass through.
		return func(c *gin.Context) { c.Next() }
	}

	netMW := auth.RequireGroupMiddleware(groupName)

	return func(c *gin.Context) {
		var passed bool

		netMW(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			c.Request = r
			passed = true
			c.Next()
		})).ServeHTTP(c.Writer, c.Request)

		if !passed {
			c.Abort()
		}
	}
}
