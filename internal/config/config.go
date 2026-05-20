package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config is the single source of truth for all runtime configuration.
// Values are read from environment variables; use .env.example as a template.
type Config struct {
	Server    ServerConfig
	AD        ADConfig
	JWT       JWTConfig
	Log       LogConfig
	Database  DatabaseConfig
	Audit     AuditConfig
	Integrity IntegrityConfig
}

type ServerConfig struct {
	Port         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	Mode         string // "debug" | "release"
}

type ADConfig struct {
	URL           string   // ldaps://dc.company.com:636
	BaseDN        string   // DC=company,DC=com
	EmployeeOU    string   // OU=Employees,DC=company,DC=com
	BindDN        string   // service account full DN
	BindPassword  string
	AdminGroupDN  string   // members of this group get the "admin" role
	EditorGroupDN string   // members of this group may update employee attributes
	TLSSkipVerify bool     // never true in production
	Attributes    []string // LDAP attributes to request
}

type JWTConfig struct {
	Secret string
	Expiry time.Duration
	Issuer string
}

type LogConfig struct {
	Level  string // "debug" | "info" | "warn" | "error"
	Format string // "json" | "console"
}

type DatabaseConfig struct {
	DSN string // SQLite file URI
}

type AuditConfig struct {
	FilePath string // LOG_FILE_PATH — JSON Lines audit log destination
	DBType   string // LOG_DB_TYPE   — "sqlite" (default) or "postgres"
	DBDSN    string // LOG_DB_DSN    — data-source name for the audit database
}

type IntegrityConfig struct {
	Enabled    bool          // INTEGRITY_ENABLED    — run the background checker (default true)
	Interval   time.Duration // INTEGRITY_INTERVAL   — period between checks (default 1h)
	AutoUpdate bool          // INTEGRITY_AUTO_UPDATE — rewrite baseline on mismatch (default false)
}

// Load reads all required and optional environment variables.
// It panics on any missing required variable so the problem surfaces at startup.
func Load() (*Config, error) {
	jwtExpiry, err := time.ParseDuration(getEnv("JWT_EXPIRY", "8h"))
	if err != nil {
		return nil, fmt.Errorf("config: invalid JWT_EXPIRY: %w", err)
	}

	integrityInterval, err := time.ParseDuration(getEnv("INTEGRITY_INTERVAL", "1h"))
	if err != nil {
		return nil, fmt.Errorf("config: invalid INTEGRITY_INTERVAL: %w", err)
	}

	return &Config{
		Server: ServerConfig{
			Port:         getEnv("SERVER_PORT", "8080"),
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			Mode:         getEnv("GIN_MODE", "release"),
		},
		AD: ADConfig{
			URL:          mustEnv("AD_URL"),
			BaseDN:       mustEnv("AD_BASE_DN"),
			EmployeeOU:   getEnv("AD_EMPLOYEE_OU", "OU=Employees,DC=company,DC=com"),
			BindDN:       mustEnv("AD_BIND_DN"),
			BindPassword: mustEnv("AD_BIND_PASSWORD"),
			AdminGroupDN:  getEnv("AD_ADMIN_GROUP", ""),
			EditorGroupDN: getEnv("AD_EDITOR_GROUP", ""),
			TLSSkipVerify: parseBool(getEnv("AD_TLS_SKIP_VERIFY", "false")),
			Attributes: []string{
				"sAMAccountName",
				"displayName",
				"mail",
				"telephoneNumber",
				"physicalDeliveryOfficeName",
				"memberOf",
			},
		},
		JWT: JWTConfig{
			Secret: mustEnv("JWT_SECRET"),
			Expiry: jwtExpiry,
			Issuer: getEnv("JWT_ISSUER", "ad-sync-manager"),
		},
		Log: LogConfig{
			Level:  getEnv("LOG_LEVEL", "info"),
			Format: getEnv("LOG_FORMAT", "json"),
		},
		Database: DatabaseConfig{
			DSN: getEnv("DATABASE_DSN", "file:./data/adsync.db?cache=shared&mode=rwc"),
		},
		Audit: AuditConfig{
			FilePath: getEnv("LOG_FILE_PATH", "./logs/audit.jsonl"),
			DBType:   getEnv("LOG_DB_TYPE", "sqlite"),
			DBDSN:    getEnv("LOG_DB_DSN", "./audit.db"),
		},
		Integrity: IntegrityConfig{
			Enabled:    parseBool(getEnv("INTEGRITY_ENABLED", "true")),
			Interval:   integrityInterval,
			AutoUpdate: parseBool(getEnv("INTEGRITY_AUTO_UPDATE", "false")),
		},
	}, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// mustEnv panics with a clear message when a required variable is absent,
// so the container exits immediately with a useful log line rather than
// failing silently later.
func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("config: required environment variable %q is not set", key))
	}
	return v
}

func parseBool(s string) bool {
	b, _ := strconv.ParseBool(s)
	return b
}
