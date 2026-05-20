package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// contextKey is an unexported type for context keys, preventing key collisions
// with other packages that also store values in request contexts.
type contextKey int

const claimsKey contextKey = iota

// ClaimsFromContext retrieves the *Claims stored by AuthMiddleware.
// Returns nil if the request has not passed through AuthMiddleware
// (e.g., a public route).
func ClaimsFromContext(ctx context.Context) *Claims {
	c, _ := ctx.Value(claimsKey).(*Claims)
	return c
}

// ContextWithClaims returns a new context with claims stored under claimsKey.
// Used by Gin adapters so that net/http-style handlers (MeHandler, etc.)
// can read claims via ClaimsFromContext regardless of which HTTP framework
// placed them there.
func ContextWithClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, claimsKey, claims)
}

// AuthMiddleware is a standard net/http middleware that:
//  1. Extracts the JWT from the "Authorization: Bearer <token>" header.
//  2. Validates it via ValidateJWT (signature, expiry, deny-list).
//  3. Stores the parsed *Claims in the request context under claimsKey.
//
// Requests with missing or invalid tokens receive 401 Unauthorized and the
// chain is halted. The response body is a JSON object with an "error" field.
//
// Usage with a plain ServeMux:
//
//	mux.Handle("/api/me", AuthMiddleware(http.HandlerFunc(MeHandler)))
//
// Usage with chi/mux router middleware chain:
//
//	r.Use(AuthMiddleware)
//
// Usage with Gin (via gin.WrapH):
//
//	ginGroup.Use(gin.WrapH(AuthMiddleware(http.HandlerFunc(noopHandler))))
//	// — or use the GinAuthMiddleware() adapter defined in internal/middleware/auth.go
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawToken := extractBearer(r.Header.Get("Authorization"))
		if rawToken == "" {
			jsonError(w, http.StatusUnauthorized, "missing or malformed Authorization header")
			return
		}

		claims, err := ValidateJWT(rawToken)
		if err != nil {
			jsonError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}

		ctx := context.WithValue(r.Context(), claimsKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireGroupMiddleware builds a middleware that allows the request only when
// the authenticated user is a member of groupName.
//
// groupName may be:
//   - A full group DN:  "CN=IT-Admins,OU=Groups,DC=company,DC=com"
//   - A CN-only name:  "IT-Admins"  (matched case-insensitively against
//     the CN component of every group DN in the user's claims)
//
// Must be chained after AuthMiddleware; returns 403 if the user lacks
// membership, 401 if claims are absent.
//
// Usage:
//
//	mux.Handle("/admin", AuthMiddleware(RequireGroupMiddleware("AD-Sync-Admins")(adminHandler)))
func RequireGroupMiddleware(groupName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := ClaimsFromContext(r.Context())
			if claims == nil {
				jsonError(w, http.StatusUnauthorized, "unauthenticated")
				return
			}

			if !userInGroup(claims.Groups, groupName) {
				jsonError(w, http.StatusForbidden, "insufficient group membership")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// extractBearer strips the "Bearer " prefix and returns the raw token.
// Returns "" if the header is absent or does not start with "Bearer ".
func extractBearer(header string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

// userInGroup reports whether any of the user's group DNs matches groupName,
// either by exact full DN or by the CN= component (case-insensitive).
func userInGroup(userGroups []string, groupName string) bool {
	for _, dn := range userGroups {
		if strings.EqualFold(dn, groupName) {
			return true
		}
		if strings.EqualFold(dnCN(dn), groupName) {
			return true
		}
	}
	return false
}

// dnCN extracts the value of the first CN= component from a DN string.
// e.g. "CN=IT-Admins,OU=Groups,DC=company,DC=com" → "IT-Admins"
func dnCN(dn string) string {
	for _, part := range strings.Split(dn, ",") {
		part = strings.TrimSpace(part)
		upper := strings.ToUpper(part)
		if strings.HasPrefix(upper, "CN=") {
			return part[3:]
		}
	}
	return ""
}

// jsonError writes a JSON {"error":"..."} response with the given status code.
func jsonError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"error":%q}`, msg)
}
