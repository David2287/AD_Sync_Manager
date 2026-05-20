-- migrations/001_create_audit_logs.sql
-- Audit log table for the AD-Sync Manager.
--
-- This migration is applied automatically by audit.NewDBLogger for SQLite.
-- For PostgreSQL deployments run this file manually before the first startup:
--   psql $DATABASE_URL < migrations/001_create_audit_logs.sql
--
-- Column notes:
--   operator   — sAMAccountName from the JWT (who performed the action)
--   action     — one of: login, apply_markdown, update_employee, integrity_check
--   target_dn  — the AD Distinguished Name of the affected object (if applicable)
--   attribute  — LDAP attribute name (for update_employee events)
--   old_value  — previous attribute value (for update events)
--   new_value  — replacement attribute value (for update events)
--   status     — "success" or "failure"
--   details    — JSON blob for extra context (error messages, operation summaries)
--   ip_address — real client IP (honouring X-Forwarded-For / X-Real-IP headers)

CREATE TABLE IF NOT EXISTS audit_logs (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp  DATETIME    NOT NULL,
    operator   TEXT        NOT NULL DEFAULT '',
    action     TEXT        NOT NULL DEFAULT '',
    target_dn  TEXT        NOT NULL DEFAULT '',
    attribute  TEXT        NOT NULL DEFAULT '',
    old_value  TEXT        NOT NULL DEFAULT '',
    new_value  TEXT        NOT NULL DEFAULT '',
    status     TEXT        NOT NULL DEFAULT '',
    details    TEXT        NOT NULL DEFAULT '',
    ip_address TEXT        NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_logs (timestamp);
CREATE INDEX IF NOT EXISTS idx_audit_operator  ON audit_logs (operator);
CREATE INDEX IF NOT EXISTS idx_audit_action    ON audit_logs (action);
