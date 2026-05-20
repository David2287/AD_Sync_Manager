package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"ad-sync-manager/internal/audit"
	"ad-sync-manager/internal/auth"
	"ad-sync-manager/internal/config"
	"ad-sync-manager/internal/employee"
	"ad-sync-manager/internal/handlers"
	"ad-sync-manager/internal/integrity"
	mdcorrect "ad-sync-manager/internal/markdown"
	"ad-sync-manager/internal/middleware"
	dbrepo "ad-sync-manager/internal/repositories/database"
	filerepo "ad-sync-manager/internal/repositories/files"
	"ad-sync-manager/internal/services"
	"ad-sync-manager/pkg/logger"
	"ad-sync-manager/pkg/markdown"
	"ad-sync-manager/web"
)

func main() {
	// ── Configuration ────────────────────────────────────────────────────────
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}

	// ── Logger ───────────────────────────────────────────────────────────────
	log, err := logger.New(cfg.Log.Level, cfg.Log.Format)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: logger init: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync() //nolint:errcheck

	// ── Auth package bootstrap ────────────────────────────────────────────────
	// auth.Init wires the package-level AD config used by AuthenticateAD,
	// GetUserGroups, and all JWT functions. Must be called before any request
	// reaches the auth middleware.
	auth.Init(cfg.AD)
	log.Info("auth package initialized", "ad_url", cfg.AD.URL, "base_dn", cfg.AD.BaseDN)

	// ── Employee package bootstrap ────────────────────────────────────────────
	// employee.Init stores the AD config used by the Stage 3 LDAP employee
	// handlers (GetAllEmployees, GetEmployeeByDN, UpdateEmployeeAttribute).
	employee.Init(cfg.AD)
	log.Info("employee package initialized", "base_dn", cfg.AD.BaseDN, "employee_ou", cfg.AD.EmployeeOU)

	// ── Markdown correction package bootstrap ─────────────────────────────────
	// mdcorrect.Init must be called after employee.Init because the default
	// ADClient adapter delegates directly to the employee package functions.
	mdcorrect.Init(mdcorrect.NewEmployeeADClient())
	log.Info("markdown correction package initialized")

	// ── Audit logger initialization ───────────────────────────────────────────
	// The file logger writes JSON Lines with lumberjack rotation (100 MB / 30 d).
	// The DB logger writes to a separate SQLite database for API-queryable logs.
	// Both are wrapped in a MultiLogger and then in an AsyncLogger so that audit
	// writes never block the HTTP request path (see AsyncLogger trade-offs).
	if err := os.MkdirAll(filepath.Dir(cfg.Audit.FilePath), 0o755); err != nil {
		log.Error("failed to create audit log directory", "error", err)
		os.Exit(1)
	}
	fileLogger, err := audit.NewFileLogger(cfg.Audit.FilePath)
	if err != nil {
		log.Error("audit file logger init failed", "error", err)
		os.Exit(1)
	}
	dbLogger, err := audit.NewDBLogger(cfg.Audit.DBType, cfg.Audit.DBDSN)
	if err != nil {
		log.Error("audit DB logger init failed", "error", err)
		os.Exit(1)
	}
	asyncAudit := audit.NewAsyncLogger(audit.NewMultiLogger(fileLogger, dbLogger), 512)
	defer asyncAudit.Close() //nolint:errcheck
	audit.Init(asyncAudit)
	log.Info("audit logger initialized",
		"file", cfg.Audit.FilePath,
		"db_type", cfg.Audit.DBType,
		"db_dsn", cfg.Audit.DBDSN)

	// ── Repositories (infrastructure layer) ─────────────────────────────────
	empRepo, err := dbrepo.NewEmployeeRepo(cfg.Database.DSN)
	if err != nil {
		log.Error("database init failed", "error", err)
		os.Exit(1)
	}

	// We still use the LDAP client from Stage 1 for employee sync operations.
	// Authentication itself now goes through the auth package directly.
	ldapClient, err := newLDAPSyncClient(cfg, log)
	if err != nil {
		log.Error("AD connection failed", "error", err)
		os.Exit(1)
	}
	defer ldapClient.Close() //nolint:errcheck

	noteRepo := filerepo.NewNoteRepo("./data/notes")
	mdParser  := markdown.NewGoldmarkParser()

	// ── Services (application / use-case layer) ──────────────────────────────
	empSvc  := services.NewEmployeeService(ldapClient, empRepo, log)
	noteSvc := services.NewNoteService(noteRepo, mdParser, log)
	syncSvc := services.NewSyncService(ldapClient, empRepo, log)

	// ── Integrity checker ─────────────────────────────────────────────────────
	// A separate connection to the same audit DB stores the integrity_baseline
	// table. The checker runs in a background goroutine and emits audit log
	// entries for every hash mismatch it detects.
	var integrityChecker *integrity.Checker
	if cfg.Integrity.Enabled {
		baselineStore, err := integrity.NewBaselineStore(cfg.Audit.DBType, cfg.Audit.DBDSN)
		if err != nil {
			log.Error("integrity baseline store init failed", "error", err)
			os.Exit(1)
		}
		defer baselineStore.Close() //nolint:errcheck

		// adActiveUsersFilter is the standard filter used by the employee package
		// (extracted here to avoid importing the unexported buildListFilter).
		const adActiveUsersFilter = "(&(objectClass=user)(objectCategory=person)" +
			"(!(userAccountControl:1.2.840.113556.1.4.803:=2)))"

		lister := func() ([]integrity.EmployeeSnapshot, error) {
			emps, _, err := employee.GetAllEmployees(
				context.Background(), cfg.AD.EmployeeOU, adActiveUsersFilter, 100_000, 0)
			if err != nil {
				return nil, err
			}
			snapshots := make([]integrity.EmployeeSnapshot, len(emps))
			for i, e := range emps {
				snapshots[i] = integrity.EmployeeSnapshot{
					DN:          e.DN,
					DisplayName: e.FullName,
					Mail:        e.Email,
					Phone:       e.TelephoneNumber,
					Office:      e.Office,
				}
			}
			return snapshots, nil
		}

		integrityChecker = integrity.NewChecker(
			baselineStore, lister, asyncAudit, cfg.Integrity.AutoUpdate)
		integrityCtx, integrityCancel := context.WithCancel(context.Background())
		defer integrityCancel()
		go integrityChecker.Start(integrityCtx, cfg.Integrity.Interval)
		log.Info("integrity checker started",
			"interval", cfg.Integrity.Interval,
			"auto_update", cfg.Integrity.AutoUpdate)
	}

	// ── Middleware ────────────────────────────────────────────────────────────
	mw := middleware.New(nil, log, cfg.AD.AdminGroupDN, cfg.AD.UseUserBind)
	// Stage 2: auth middleware uses the auth package directly (no SessionService).

	// ── HTTP router ───────────────────────────────────────────────────────────
	router := handlers.NewRouter(handlers.RouterDeps{
		Employee:         empSvc,
		Note:             noteSvc,
		Sync:             syncSvc,
		Middleware:       mw,
		Logger:           log,
		Mode:             cfg.Server.Mode,
		AdminGroupDN:     cfg.AD.AdminGroupDN,
		EditorGroupDN:    cfg.AD.EditorGroupDN,
		UseUserBind:      cfg.AD.UseUserBind,
		AuditQuerier:     dbLogger,
		IntegrityChecker: integrityChecker,
		StaticFS:         web.FS,
	})

	srv := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// ── Start + graceful shutdown ─────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Info("AD-Sync Manager started", "port", cfg.Server.Port, "mode", cfg.Server.Mode)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-quit
	log.Info("shutting down gracefully…")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}

	log.Info("server stopped")
}
