package auth

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const tokenExpiry = 8 * time.Hour

// Claims is the verified JWT payload embedded into every authenticated request.
// It embeds jwt.RegisteredClaims so standard fields (exp, iss, sub, iat) are
// handled automatically by the JWT library.
type Claims struct {
	Username string   `json:"username"` // sAMAccountName
	DN       string   `json:"dn"`       // full Distinguished Name
	Groups   []string `json:"groups"`   // list of group DNs at login time
	jwt.RegisteredClaims
}

// ── Token deny-list ───────────────────────────────────────────────────────────
// Tokens are added here on logout. The map is keyed by the raw token string.
// Stage 1 keeps this in-memory; replace with a Redis SET (with TTL = remaining
// token lifetime) before deploying to a multi-instance environment.

var (
	revokedMu sync.RWMutex
	revokedTokens = map[string]struct{}{}
)

// RevokeToken adds a raw JWT string to the in-process deny-list.
// Subsequent calls to ValidateJWT with the same token return an error.
func RevokeToken(rawToken string) {
	revokedMu.Lock()
	revokedTokens[rawToken] = struct{}{}
	revokedMu.Unlock()
}

func isRevoked(rawToken string) bool {
	revokedMu.RLock()
	_, ok := revokedTokens[rawToken]
	revokedMu.RUnlock()
	return ok
}

// ── Public helpers ────────────────────────────────────────────────────────────

// GenerateJWT creates and signs an HS256 JWT for the given user.
//
// Expiration is fixed at 8 hours (tokenExpiry).
// The signing secret is read from the JWT_SECRET environment variable.
// JWT_SECRET must be at least 32 characters; startup will fail otherwise.
func GenerateJWT(username, dn string, groups []string) (string, error) {
	secret, err := loadSecret()
	if err != nil {
		return "", err
	}

	now := time.Now()
	claims := &Claims{
		Username: username,
		DN:       dn,
		Groups:   groups,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "ad-sync-manager",
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(tokenExpiry)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(secret)
	if err != nil {
		return "", fmt.Errorf("jwt: sign: %w", err)
	}
	return signed, nil
}

// ValidateJWT parses a raw JWT string, verifies:
//   - HMAC-SHA256 signature
//   - expiry (exp claim)
//   - issuer ("ad-sync-manager")
//   - not in deny-list (revoked on logout)
//
// Returns the embedded Claims on success.
func ValidateJWT(tokenString string) (*Claims, error) {
	if isRevoked(tokenString) {
		return nil, fmt.Errorf("jwt: token has been revoked")
	}

	secret, err := loadSecret()
	if err != nil {
		return nil, err
	}

	token, err := jwt.ParseWithClaims(
		tokenString,
		&Claims{},
		func(t *jwt.Token) (any, error) {
			// Reject anything that isn't HS256 — prevents "alg:none" attacks.
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("jwt: unexpected signing algorithm %v", t.Header["alg"])
			}
			return secret, nil
		},
		jwt.WithExpirationRequired(),
		jwt.WithIssuer("ad-sync-manager"),
	)
	if err != nil {
		return nil, fmt.Errorf("jwt: validation failed: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("jwt: malformed claims payload")
	}
	return claims, nil
}

// loadSecret reads JWT_SECRET from the environment and enforces a minimum length.
// 32 bytes gives 256 bits of entropy, sufficient for HS256.
func loadSecret() ([]byte, error) {
	s := os.Getenv("JWT_SECRET")
	if len(s) < 32 {
		return nil, fmt.Errorf("jwt: JWT_SECRET must be at least 32 characters (got %d)", len(s))
	}
	return []byte(s), nil
}
