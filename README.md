# AD Sync Manager

A self-contained Go web application for managing Active Directory employee records. It provides authenticated LDAP synchronisation, a markdown-based bulk correction workflow, a dual-backend audit trail, integrity monitoring, and a React web UI — all served from a single statically-linked binary.

---

## Features

- **AD Authentication** — users log in with their AD credentials; JWT is issued and scoped to admin / editor roles via group membership
- **Employee Directory** — paginated, searchable table synced from LDAP; editors can update `office` and `telephoneNumber` inline
- **Markdown Corrections** — upload a markdown document describing field corrections; validate before applying; every change is audited
- **Audit Logging** — dual-backend (JSON Lines file + SQLite); filterable log viewer with CSV export
- **Integrity Monitoring** — background checker compares live LDAP data against a stored baseline; alerts on drift
- **React Web UI** — dark/light themed SPA embedded directly in the binary; no separate static server needed

---

## Architecture

```
cmd/server/         ← application entry point
internal/
  auth/             ← LDAP authentication, JWT issuance
  config/           ← environment-variable configuration
  employee/         ← LDAP employee sync, cache, HTTP handlers
  markdown/         ← markdown parse, validate, apply
  audit/            ← dual-backend audit logger + query API
  integrity/        ← baseline checker + HTTP handlers
  middleware/        ← JWT auth, RBAC (RequireGroup), request logging
  handlers/          ← Gin router wiring
  domain/interfaces/ ← service contracts
  repositories/      ← SQLite repositories
  services/          ← business-logic services
migrations/         ← SQL schema files
frontend/           ← React + TypeScript + Vite source
web/                ← Go embed target (frontend/vite.config.ts → ../web/dist)
deployments/        ← Dockerfile, docker-compose.yml
```

---

## Quick Start

### Prerequisites

- Go 1.21+
- Node.js 20+ (for UI development / first-time build)
- Access to an Active Directory / LDAP server

### 1. Configure

Copy `.env.example` to `.env` and fill in required values:

```env
# Required
AD_URL=ldaps://dc.company.com:636
AD_BASE_DN=DC=company,DC=com
AD_BIND_DN=CN=svc-adsync,OU=ServiceAccounts,DC=company,DC=com
AD_BIND_PASSWORD=supersecret
JWT_SECRET=change-me-to-a-long-random-string

# Optional (defaults shown)
SERVER_PORT=8080
AD_EMPLOYEE_OU=OU=Employees,DC=company,DC=com
AD_ADMIN_GROUP=CN=ADSyncAdmins,OU=Groups,DC=company,DC=com
AD_EDITOR_GROUP=CN=ADSyncEditors,OU=Groups,DC=company,DC=com
JWT_EXPIRY=8h
LOG_LEVEL=info
LOG_FORMAT=json
DATABASE_DSN=file:./data/adsync.db?cache=shared&mode=rwc
LOG_FILE_PATH=./logs/audit.jsonl
LOG_DB_DSN=./audit.db
INTEGRITY_ENABLED=true
INTEGRITY_INTERVAL=1h
INTEGRITY_AUTO_UPDATE=false
```

### 2. Build

```bash
# Build the frontend first (only needed once, or after UI changes)
cd frontend && npm install && npm run build && cd ..

# Build the Go binary (embeds web/dist at compile time)
go build -o ad-sync-manager ./cmd/server
```

### 3. Run

```bash
./ad-sync-manager
# Open http://localhost:8080
```

---

## Docker

```bash
# Build and start (docker-compose handles the 3-stage build automatically)
docker compose -f deployments/docker-compose.yml up -d

# View logs
docker compose -f deployments/docker-compose.yml logs -f
```

The Dockerfile is a 3-stage build:

1. **Node 20 Alpine** — compiles the React frontend
2. **Go 1.21 Alpine** — embeds the compiled frontend and builds the binary (CGO disabled)
3. **Alpine runtime** — minimal image, non-root user

---

## Development

```bash
# Terminal 1 — Go backend
go run ./cmd/server

# Terminal 2 — Vite dev server (hot reload, proxies /api → :8080)
cd frontend && npm run dev
# Open http://localhost:3000
```

---

## API Reference

All endpoints are under `/api/v1`. Authentication is via `Authorization: Bearer <token>`.

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `POST` | `/auth/login` | public | AD login → JWT |
| `POST` | `/auth/logout` | auth | Invalidate session |
| `GET` | `/me` | auth | Current user profile |
| `GET` | `/me/perms` | auth | `{isAdmin, isEditor}` for the current user |
| `GET` | `/employees` | auth | List employees (`limit`, `offset`, `search`) |
| `GET` | `/employees/:id` | auth | Get single employee by URL-encoded DN |
| `PUT` | `/employees/:id` | editor | Update `telephoneNumber` / `physicalDeliveryOfficeName` |
| `POST` | `/markdown/validate` | auth | Parse and validate a corrections document |
| `POST` | `/markdown/apply` | editor | Apply validated corrections to AD |
| `POST` | `/sync` | admin | Trigger a manual LDAP sync |
| `GET` | `/sync/status` | admin | Sync status |
| `GET` | `/logs` | admin | List audit logs (`operator`, `action`, `status`, `from`, `to`) |
| `GET` | `/logs/:id` | admin | Single audit log entry |
| `GET` | `/integrity/report` | admin | Current integrity report |
| `POST` | `/integrity/reset` | admin | Reset the integrity baseline |
| `GET` | `/healthz` | public | Health probe |

---

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `AD_URL` | yes | — | LDAP/LDAPS server URL. Always use `ldaps://` in production |
| `AD_BASE_DN` | yes | — | Base distinguished name |
| `AD_BIND_DN` | yes | — | Service account DN for LDAP bind |
| `AD_BIND_PASSWORD` | yes | — | Service account password |
| `JWT_SECRET` | yes | — | HMAC-SHA256 signing key for JWTs |
| `AD_EMPLOYEE_OU` | no | `OU=Employees,DC=company,DC=com` | OU to search for employees |
| `AD_ADMIN_GROUP` | no | `""` | Full DN of the admin group (empty = no admin-gating) |
| `AD_EDITOR_GROUP` | no | `""` | Full DN of the editor group |
| `AD_TLS_SKIP_VERIFY` | no | `false` | Skip TLS cert verification (never `true` in production) |
| `JWT_EXPIRY` | no | `8h` | Token lifetime (Go duration string) |
| `JWT_ISSUER` | no | `ad-sync-manager` | JWT `iss` claim |
| `SERVER_PORT` | no | `8080` | HTTP listen port |
| `GIN_MODE` | no | `release` | Gin mode (`debug` \| `release`) |
| `LOG_LEVEL` | no | `info` | Structured log level |
| `LOG_FORMAT` | no | `json` | Log format (`json` \| `console`) |
| `DATABASE_DSN` | no | `file:./data/adsync.db?…` | SQLite DSN for the main database |
| `LOG_FILE_PATH` | no | `./logs/audit.jsonl` | JSON Lines audit log file path |
| `LOG_DB_TYPE` | no | `sqlite` | Audit database type |
| `LOG_DB_DSN` | no | `./audit.db` | Audit database DSN |
| `INTEGRITY_ENABLED` | no | `true` | Enable background integrity checker |
| `INTEGRITY_INTERVAL` | no | `1h` | Interval between integrity checks |
| `INTEGRITY_AUTO_UPDATE` | no | `false` | Automatically update baseline on mismatch |

---

## Security Notes

- Always use `ldaps://` (port 636) for AD connections in production
- Passwords are never logged; only successful/failed login attempts are recorded
- API error messages are intentionally generic to prevent information leakage
- The JWT secret must be set via environment variable (`JWT_SECRET`); it is never committed to source control
- The Docker image runs as a non-root user (`adsync`)

---

## Testing

```bash
# Unit and integration tests
go test ./...

# With race detector
go test -race ./...

# Integration tests require a running LDAP server (see deployments/docker-compose.test.yml)
docker compose -f deployments/docker-compose.test.yml up -d
go test -tags integration ./...
```

---

## Project Status

All 7 development stages are complete:

| Stage | Description |
|-------|-------------|
| 1 | AD authentication (LDAPS, JWT, RBAC middleware) |
| 2 | Employee sync (LDAP read, pagination, search, caching) |
| 3 | Markdown corrections (parse, validate, apply) |
| 4 | Response caching (in-memory + cache-control headers) |
| 5 | Audit logging (dual-backend: JSON Lines + SQLite, async writer) |
| 6 | Testing & integrity checking (unit + integration tests, baseline checker) |
| 7 | React web UI (React 18 + TypeScript + Tailwind, embedded in binary) |
"# AD_Sync_Manager" 
