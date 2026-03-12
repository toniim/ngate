# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What is Ngate

Ngate is a lightweight Nginx reverse proxy gateway — a single Go binary that provides a web admin UI and REST API for managing Nginx reverse proxy sites with automatic SSL certificate provisioning (Let's Encrypt via certbot, or mkcert for local dev).

## Build & Run Commands

```bash
# Build (CGO required for SQLite)
CGO_ENABLED=1 go build -o bin/ngate ./main.go
# or
make build

# Dev mode (uses /tmp paths, requires local nginx)
make dev

# Run tests
go test -race -cover -v -count=1 -timeout=30s ./...

# Docker
make docker-build   # docker compose build
make docker-up      # docker compose up -d
make docker-down    # docker compose down
make docker-logs    # docker compose logs -f

# Verify nginx config
sudo nginx -t
```

## Architecture

Single-binary monolith with embedded SPA frontend:

```
Browser → :8080 → chi router
                    ├── /api/*  → api.Handler → DB (SQLite) + Nginx (exec) + CertManager (exec)
                    └── /*      → embedded static UI (go:embed)
```

**Request flow for site creation:** API handler validates input → writes to SQLite → generates nginx config from Go template → runs `nginx -t` → reloads nginx → if SSL requested, issues cert async in goroutine → regenerates config with SSL → reloads nginx again.

### Key packages

- `internal/api` — REST handlers + chi route registration. All handler methods on `Handler` struct which holds concrete deps (no interfaces).
- `internal/db` — SQLite via `database/sql` + `go-sqlite3` (CGO). Inline schema migration at startup.
- `internal/nginx` — Generates per-site nginx `.conf` files from `text/template`, tests and reloads via `exec.Command`.
- `internal/certmanager` — Issues TLS certs via certbot (production) or mkcert (local dev). Checks cert existence/expiry.
- `internal/models` — Data structs (`Site`, `AppConfig`) and SSL/proxy type constants.
- `ui/static/index.html` — Single-file SPA embedded at compile time.

## Important Details

- **CGO is mandatory** — `go-sqlite3` requires a C compiler. Cross-compilation needs a C toolchain.
- **sudo required** — nginx operations need root. `make run` and `make dev` use sudo.
- **Async cert issuance** — SSL certs are issued in a goroutine; site initially serves HTTP-only, then config is regenerated after cert is ready.
- **No auth** — Admin API and UI have no authentication. CORS allows all origins.
- **Runtime flags:** `-port` (8080), `-data` (/etc/ngate), `-conf` (/etc/nginx/sites-enabled), `-certs` (/etc/ngate/certs).
- **No test files exist yet.**
