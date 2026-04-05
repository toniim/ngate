# Ngate

Lightweight Nginx proxy gateway with a web UI, written in Go.

## Features

- **Web UI** — Add/edit/delete sites with domain, proxy target, or static file serving
- **SSL Management** — Auto-issue via Let's Encrypt (production) or mkcert (local dev)
- **Nginx Control** — Auto-generates configs, tests, and hot-reloads Nginx
- **Force HTTPS** — Optional HTTP→HTTPS redirect per site
- **Custom Directives** — Inject extra Nginx config per site
- **Single Binary** — UI embedded in the Go binary, no Node.js needed

## Architecture

```
┌─────────────────┐     ┌──────────────────┐
│  Browser :8080  │────▶│  Go API Server   │
│  (Admin UI)     │     │  - REST API      │
└─────────────────┘     │  - Embedded UI   │
                        │  - Cert Manager  │
                        └────────┬─────────┘
                                 │ generates configs
                                 ▼
                        ┌──────────────────┐
                        │     Nginx        │
                        │  :80 / :443      │
                        │  sites-enabled/  │
                        └──────────────────┘
                                 │
                    ┌────────────┼────────────┐
                    ▼            ▼            ▼
              app1:3000    app2:8000    /var/www/site
```

## Quick Start (Docker)

```bash
# Clone & run
git clone <repo>
cd nginx-ngate

# Build & start
docker compose up -d

# Open UI
open http://localhost:8080
```

## Development

```bash
# Dev mode with hot reload (Docker + air)
make dev

# Stop dev container
make dev-down
```

Edit any `.go` or `.html` file and air will auto-rebuild inside the container.

## Deploy to VPS

`make deploy` rsyncs source to the remote host and rebuilds the Docker image there (multi-stage build, no local Go toolchain needed).

```bash
# Deploy (default host from Makefile: HOST=tonysproxy)
make deploy

# Deploy to a different host
make deploy HOST=my-vps
```

**Host configuration:** `HOST` must be an SSH host defined in `~/.ssh/config`:

```
Host my-vps
  Hostname 1.2.3.4
  User ubuntu
  IdentityFile ~/.ssh/id_rsa
```

**What happens on deploy:**
1. `rsync` syncs source to `~/ngate/` on the remote host (excludes `data/`, `.git/`, `bin/`)
2. `docker compose up -d --build` runs multi-stage Go build and restarts the container

**Data is safe across deploys** — `data/` lives on the host via Docker volume, not inside the image.

## Data & Backup

All persistent data is stored on the host under `./data/ngate/` (mounted to `/etc/ngate` in the container):

```
./data/
├── ngate/
│   ├── proxy-manager.db     # SQLite — sites, certs, providers config
│   └── certs/
│       ├── letsencrypt/     # Let's Encrypt certs (per domain)
│       └── mkcert/          # mkcert certs (local dev)
└── acme/                    # ACME challenge files (ephemeral)
```

Nginx configs (`/etc/nginx/sites-enabled/`) live inside the container and are **not persisted** — they are regenerated from the database on every startup.

**Backup:** copy `./data/ngate/` — that's everything (DB + certs).

```bash
# Backup
tar czf ngate-backup-$(date +%Y%m%d).tar.gz ./data/ngate/

# Restore on a new host
tar xzf ngate-backup-*.tar.gz
make deploy HOST=new-vps
```

## Usage Guide

All configuration is done through the Web UI (default `:8090` on host, `:8080` in container).

### 1. Cert Providers

A cert provider defines **how** certificates are issued. Create one before issuing any certificate.

| Type | Use case | Config required |
|------|----------|-----------------|
| `mkcert` | Local dev, internal networks | None |
| `letsencrypt_http01` | Public sites (port 80 must be open) | `email` |
| `letsencrypt_dns01_cloudflare` | Wildcard certs, Cloudflare DNS | `email`, `api_token` |
| `letsencrypt_dns01_route53` | Wildcard certs, AWS Route53 | `email`, `access_key_id`, `secret_access_key`, `region` |

Config can be entered as YAML in the UI:

```yaml
email: admin@example.com
api_token: cf-api-token-here
staging: false
```

### 2. Certificates

A certificate is issued by a provider for a domain (+ optional alt/wildcard domains).

- **Domain:** primary domain (e.g. `example.com`)
- **Alt Domains (SANs):** comma-separated, e.g. `*.example.com, api.example.com`
- **Wildcard certs** require a DNS-01 provider (Cloudflare or Route53)

Cert issuance is async — status transitions: `pending` → `issuing` → `active` or `error`. The UI auto-polls until resolved.

To retry a failed cert, click the refresh button on the certificate card.

### 3. Sites

A site maps a domain to a backend (reverse proxy) or filesystem path (static files).

| Field | Description |
|-------|-------------|
| **Domain** | The hostname nginx listens for (e.g. `app.example.com`) |
| **Type** | `reverse_proxy` or `static` |
| **Proxy Target** | Backend URL, e.g. `http://10.0.0.1:3000` (for reverse proxy) |
| **Static Root** | Filesystem path, e.g. `/var/www/site` (for static) |
| **Certificate** | Select an issued certificate (or None for HTTP only) |
| **Force HTTPS** | Redirect HTTP → HTTPS (requires a certificate) |
| **Custom Nginx** | Extra directives injected inside the `server {}` block |
| **Enabled** | Toggle site on/off without deleting |

The default reverse proxy template already includes WebSocket support (`Upgrade` + `Connection` headers), so no custom directives needed for WS.

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/sites` | List all sites |
| `POST` | `/api/sites` | Create a site |
| `GET` | `/api/sites/:id` | Get site details |
| `PUT` | `/api/sites/:id` | Update a site |
| `DELETE` | `/api/sites/:id` | Delete a site |
| `POST` | `/api/sites/:id/enable` | Toggle enable/disable |
| `POST` | `/api/sites/:id/ssl/issue` | Issue/renew SSL cert |
| `GET` | `/api/nginx/status` | Check nginx config status |
| `POST` | `/api/nginx/reload` | Reload nginx |
| `GET` | `/api/mkcert/caroot` | Get mkcert CA root + trust commands |

### Example: Add a site via API

```bash
curl -X POST http://localhost:8080/api/sites \
  -H "Content-Type: application/json" \
  -d '{
    "domain": "app.local",
    "proxy_type": "reverse_proxy",
    "proxy_target": "http://localhost:3000",
    "ssl_mode": "mkcert",
    "force_https": true,
    "enabled": true
  }'
```

## SSL Modes

### Let's Encrypt (Production)

- Domain must be publicly accessible on port 80
- Uses certbot webroot validation (`/var/www/acme`)
- Auto-renewal can be set up via cron: `0 0 * * * certbot renew`

### mkcert (Local Development)

- Creates locally-trusted certificates
- Clients must trust the mkcert root CA

#### Trust mkcert CA on Clients

**macOS:**
```bash
# Copy rootCA.pem from server, then:
sudo security add-trusted-cert -d -r trustRoot \
  -k /Library/Keychains/System.keychain rootCA.pem
```

**Windows (Admin PowerShell):**
```powershell
certutil -addstore -f "ROOT" rootCA.pem
```

**Linux:**
```bash
sudo cp rootCA.pem /usr/local/share/ca-certificates/mkcert-ca.crt
sudo update-ca-certificates
```

**Browser-specific (Chrome/Edge):**
```
Settings → Privacy & Security → Security → Manage certificates → Import rootCA.pem
```

**Firefox:**
```
Settings → Privacy & Security → View Certificates → Authorities → Import
```

> **Tip:** In the UI, click "🔐 Trust CA" → "Detect CA Path" to get the exact commands for your server.

## Configuration

| Flag | Default | Description |
|------|---------|-------------|
| `-port` | `8080` | Admin UI port |
| `-data` | `/etc/ngate` | Data directory (SQLite DB) |
| `-conf` | `/etc/nginx/sites-enabled` | Nginx config output directory |
| `-certs` | `/etc/ngate/certs` | Certificate storage directory |

## Directory Structure

```
nginx-ngate/
├── main.go                  # Entry point (embeds UI)
├── internal/
│   ├── api/handler.go       # REST API handlers
│   ├── certmanager/         # Let's Encrypt + mkcert
│   ├── db/db.go             # SQLite storage
│   ├── models/models.go     # Data types
│   └── nginx/nginx.go       # Config generator & reload
├── ui/static/index.html     # Embedded web UI
├── nginx/nginx.conf         # Base nginx config
├── scripts/entrypoint.sh    # Docker entrypoint
├── Dockerfile
├── docker-compose.yml
└── Makefile
```

## License

MIT
