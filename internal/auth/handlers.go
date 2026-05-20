package auth

import (
	"encoding/json"
	"net/http"
)

// ── Request / Response DTOs ───────────────────────────────────────────────────

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token    string `json:"token"`
	Username string `json:"username"`
	DN       string `json:"dn"`
}

// MeResponse is exported so Gin handlers and tests can reference the shape.
type MeResponse struct {
	Username string   `json:"username"`
	DN       string   `json:"dn"`
	Groups   []string `json:"groups"`
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// LoginHandler handles POST /api/login.
//
// Expects a JSON body: {"username": "...", "password": "..."}
// On success returns HTTP 200 with {"token": "...", "username": "...", "dn": "..."}.
//
// Error responses use generic messages to avoid leaking account information:
//   - 400: malformed JSON or missing fields
//   - 401: invalid credentials (wrong username or password)
//   - 500: LDAP / infrastructure unavailable (logged server-side, not exposed)
//
// Passwords are never logged. The handler delegates all AD logic to
// AuthenticateAD, which is independently testable.
func LoginHandler(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid JSON body"))
		return
	}

	if req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, errBody("username and password are required"))
		return
	}

	ok, dn, groups, err := AuthenticateAD(req.Username, req.Password)
	if err != nil {
		// Infrastructure failure — do not expose internals to the client.
		// The real error should be logged by the caller / middleware.
		writeJSON(w, http.StatusServiceUnavailable, errBody("authentication service unavailable"))
		return
	}
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errBody("invalid credentials"))
		return
	}

	_ = groups // groups are embedded in the JWT, not returned in the response body

	token, err := GenerateJWT(req.Username, dn, groups)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody("could not issue token"))
		return
	}

	writeJSON(w, http.StatusOK, loginResponse{
		Token:    token,
		Username: req.Username,
		DN:       dn,
	})
}

// MeHandler handles GET /api/me.
//
// Requires a valid JWT; returns the authenticated user's profile including
// all group DNs that were current at login time.
// Must be served behind AuthMiddleware.
func MeHandler(w http.ResponseWriter, r *http.Request) {
	claims := ClaimsFromContext(r.Context())
	if claims == nil {
		// Should not reach here if AuthMiddleware is applied, but be defensive.
		writeJSON(w, http.StatusUnauthorized, errBody("unauthenticated"))
		return
	}

	writeJSON(w, http.StatusOK, MeResponse{
		Username: claims.Username,
		DN:       claims.DN,
		Groups:   claims.Groups,
	})
}

// ── helpers ───────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func errBody(msg string) map[string]string {
	return map[string]string{"error": msg}
}
