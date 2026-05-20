package handlers

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"ad-sync-manager/internal/audit"
	"ad-sync-manager/internal/auth"
	"ad-sync-manager/internal/employee"
	httphandlers "ad-sync-manager/internal/handlers/http"
	"ad-sync-manager/internal/integrity"
	mdcorrect "ad-sync-manager/internal/markdown"
	"ad-sync-manager/internal/middleware"
	"ad-sync-manager/internal/domain/interfaces"
)

// RouterDeps carries all handler and middleware dependencies.
type RouterDeps struct {
	Employee         interfaces.EmployeeService
	Note             interfaces.NoteService
	Sync             interfaces.SyncService
	Middleware       *middleware.Bundle
	Logger           interfaces.Logger
	Mode             string // "debug" | "release"
	AdminGroupDN     string // AD group whose members have admin access
	EditorGroupDN    string // AD group whose members may update employee attributes
	AuditQuerier     audit.AuditQuerier        // nil if DB audit logging is not configured
	IntegrityChecker *integrity.Checker        // nil if integrity checking is disabled
	// StaticFS is the embedded frontend build. When non-nil, the router serves
	// the React SPA at / and falls back to index.html for unknown routes.
	StaticFS         fs.FS
}

// NewRouter wires Gin routes to their handlers.
//
// Route table:
//
//	POST   /api/login                     → login             (public, legacy path)
//	POST   /api/v1/auth/login             → login             (public)
//	POST   /api/v1/auth/logout            → logout            [auth]
//	GET    /api/me                        → me                [auth] (legacy path)
//	GET    /api/v1/me                     → me                [auth]
//	GET    /api/v1/employees              → list              [auth]         — LDAP, cached, paginated
//	GET    /api/v1/employees/:id          → get by DN         [auth]         — LDAP, cached  (:id = URL-encoded DN)
//	PUT    /api/v1/employees/:id          → update attrs      [auth][editor] — LDAP ModifyRequest
//	GET    /api/v1/employees/:id/notes    → list notes        [auth]
//	POST   /api/v1/employees/:id/notes    → create note       [auth]
//	PUT    /api/v1/notes/:id              → update note       [auth]
//	DELETE /api/v1/notes/:id              → delete note       [auth]
//	POST   /api/v1/markdown/validate      → validate doc      [auth]
//	POST   /api/v1/markdown/apply         → apply corrections [auth][editor]
//	POST   /api/v1/sync                   → run sync          [auth][admin]
//	GET    /api/v1/sync/status            → sync status       [auth][admin]
//	GET    /api/v1/logs                   → list audit logs   [auth][admin]
//	GET    /api/v1/logs/:id              → get audit log     [auth][admin]
//	GET    /api/v1/integrity/report       → integrity report  [auth][admin]
//	POST   /api/v1/integrity/reset        → reset baseline    [auth][admin]
//	GET    /healthz                       → probe             (public)
func NewRouter(deps RouterDeps) *gin.Engine {
	gin.SetMode(deps.Mode)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(deps.Middleware.RequestLogger())

	// ── Health probe ──────────────────────────────────────────────────────────
	r.GET("/healthz", func(c *gin.Context) { c.Status(http.StatusOK) })

	authHandler := httphandlers.NewAuthHandler()

	// ── Public login (both paths for backwards-compatibility) ─────────────────
	r.POST("/api/login", authHandler.Login)

	v1 := r.Group("/api/v1")
	v1.POST("/auth/login", authHandler.Login)

	// ── Authenticated group ───────────────────────────────────────────────────
	secured := v1.Group("/")
	secured.Use(deps.Middleware.RequireAuth())
	{
		secured.POST("/auth/logout", authHandler.Logout)

		// /api/v1/me and the legacy /api/me path
		secured.GET("/me", authHandler.Me)
		// Also register the legacy /api/me path on the root group with auth
		r.GET("/api/me", deps.Middleware.RequireAuth(), authHandler.Me)

		// /api/v1/me/perms returns computed isAdmin/isEditor flags for the UI.
		secured.GET("/me/perms", func(c *gin.Context) {
			claims := auth.ClaimsFromContext(c.Request.Context())
			if claims == nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"isAdmin":  groupMatches(claims.Groups, deps.AdminGroupDN),
				"isEditor": groupMatches(claims.Groups, deps.EditorGroupDN),
			})
		})

		// Wrap the framework-agnostic LoginHandler and MeHandler for direct
		// net/http compatibility testing:
		// secured.GET("/me-compat", gin.WrapF(auth.MeHandler))
		// (uncomment if you need to verify the stdlib handlers in the Gin stack)
		_ = auth.LoginHandler // ensure the symbol is reachable
		_ = auth.MeHandler

		// Stage 3: LDAP-backed employee endpoints with in-memory caching.
		// employee.Init must have been called at startup (see main.go).
		// The :id path parameter holds a URL-encoded Distinguished Name.  The same
		// wildcard name (:id) is used for both the employee routes and the existing
		// notes sub-resource routes so that Gin's radix tree stays conflict-free.
		// injectGinParam copies the Gin path param into the "dn" query key that
		// the plain net/http employee handlers read via r.URL.Query().Get("dn").
		secured.GET("/employees", gin.WrapF(employee.ListEmployeesHandler))
		secured.GET("/employees/:id", func(c *gin.Context) {
			injectGinParam(c, "id", "dn", employee.GetEmployeeHandler)
		})

		editor := secured.Group("/")
		editor.Use(deps.Middleware.RequireGroup(deps.EditorGroupDN))
		{
			editor.PUT("/employees/:id", func(c *gin.Context) {
				injectGinParam(c, "id", "dn", employee.UpdateEmployeeHandler)
			})
		}

		notes := httphandlers.NewNoteHandler(deps.Note)
		secured.GET("/employees/:id/notes", notes.ListForEmployee)
		secured.POST("/employees/:id/notes", notes.Create)
		secured.PUT("/notes/:id", notes.Update)
		secured.DELETE("/notes/:id", notes.Delete)

		// ── Markdown correction workflow ──────────────────────────────────────
		// /validate is open to all authenticated users (read-only — no AD writes).
		// /apply requires editor group membership (same gate as PUT /employees/:id).
		secured.POST("/markdown/validate", gin.WrapF(mdcorrect.ValidateMarkdownHandler))
		mdEditor := secured.Group("/")
		mdEditor.Use(deps.Middleware.RequireGroup(deps.EditorGroupDN))
		mdEditor.POST("/markdown/apply", gin.WrapF(mdcorrect.ApplyMarkdownHandler))

		// ── Admin-only ────────────────────────────────────────────────────────
		admin := secured.Group("/")
		admin.Use(deps.Middleware.RequireAdmin())
		{
			sync := httphandlers.NewSyncHandler(deps.Sync)
			admin.POST("/sync", sync.Run)
			admin.GET("/sync/status", sync.Status)

			logs := httphandlers.NewLogsHandler(deps.AuditQuerier)
			admin.GET("/logs", logs.List)
			admin.GET("/logs/:id", logs.GetByID)

			if deps.IntegrityChecker != nil {
				intHandler := integrity.NewIntegrityHandler(deps.IntegrityChecker)
				admin.GET("/integrity/report", intHandler.GetReport)
				admin.POST("/integrity/reset", intHandler.ResetBaseline)
			}
		}
	}

	// ── SPA static file serving ───────────────────────────────────────────────
	// When StaticFS is provided (production binary), serve the React build at /.
	// Requests for unknown paths that do NOT start with /api fall back to
	// index.html so that client-side routing (React Router) works correctly.
	if deps.StaticFS != nil {
		distFS, err := fs.Sub(deps.StaticFS, "dist")
		if err == nil {
			staticHandler := http.FileServer(http.FS(distFS))
			r.NoRoute(func(c *gin.Context) {
				path := c.Request.URL.Path
				// Let API routes surface their own 404 via the normal handler chain.
				if strings.HasPrefix(path, "/api/") {
					c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
					return
				}
				// For static assets that actually exist (JS, CSS, favicon) serve them.
				// For all other paths, return index.html for client-side routing.
				if _, err := fs.Stat(distFS, strings.TrimPrefix(path, "/")); err == nil {
					staticHandler.ServeHTTP(c.Writer, c.Request)
				} else {
					c.Request.URL.Path = "/"
					staticHandler.ServeHTTP(c.Writer, c.Request)
				}
			})
		}
	}

	return r
}

// groupMatches returns true when any entry in groups matches target.
// Accepts a full DN match (case-insensitive) or a CN-only match.
// Returns true when target is empty (no restriction configured).
func groupMatches(groups []string, target string) bool {
	if target == "" {
		return true
	}
	target = strings.ToLower(strings.TrimSpace(target))
	targetCN := extractCN(target)
	for _, g := range groups {
		g = strings.ToLower(strings.TrimSpace(g))
		if g == target {
			return true
		}
		if targetCN != "" && extractCN(g) == targetCN {
			return true
		}
	}
	return false
}

// extractCN returns the CN value from a DN string, or "" if not found.
func extractCN(dn string) string {
	for _, part := range strings.Split(dn, ",") {
		if after, ok := strings.CutPrefix(strings.TrimSpace(strings.ToLower(part)), "cn="); ok {
			return after
		}
	}
	return ""
}

// injectGinParam extracts the Gin path parameter named ginParam, stores it
// under queryKey in the request URL query string, then calls the plain
// net/http handler. This bridges Gin's routing layer with the framework-
// agnostic employee handlers, which read r.URL.Query().Get(queryKey).
//
// Example: ginParam="id", queryKey="dn" — the Gin route is registered as
// /employees/:id (consistent with notes sub-resources) but the employee
// handler reads r.URL.Query().Get("dn"). Gin URL-decodes c.Param before
// returning it, so the handler receives the raw DN string directly.
func injectGinParam(c *gin.Context, ginParam, queryKey string, h http.HandlerFunc) {
	q := c.Request.URL.Query()
	q.Set(queryKey, c.Param(ginParam))
	c.Request.URL.RawQuery = q.Encode()
	h(c.Writer, c.Request)
}
