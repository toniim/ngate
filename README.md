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

No auth. CORS allows all origins. Base path: `/api`.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/sites` | List sites |
| `POST` | `/api/sites` | Create site |
| `GET` | `/api/sites/:id` | Get site |
| `PUT` | `/api/sites/:id` | Update site |
| `DELETE` | `/api/sites/:id` | Delete site |
| `POST` | `/api/sites/:id/enable` | Toggle enabled |
| `GET` | `/api/cert-providers` | List cert providers |
| `POST` | `/api/cert-providers` | Create provider |
| `GET` | `/api/cert-providers/:id` | Get provider |
| `PUT` | `/api/cert-providers/:id` | Update provider |
| `DELETE` | `/api/cert-providers/:id` | Delete provider |
| `GET` | `/api/certificates` | List certificates |
| `POST` | `/api/certificates` | Issue certificate |
| `GET` | `/api/certificates/:id` | Get certificate |
| `DELETE` | `/api/certificates/:id` | Delete certificate |
| `POST` | `/api/certificates/:id/renew` | Renew certificate |
| `GET` | `/api/nginx/status` | Nginx config status |
| `POST` | `/api/nginx/reload` | Reload nginx |
| `GET` | `/api/mkcert/caroot` | mkcert CA root + trust commands |
| `GET` | `/api/mkcert/rootca.pem` | Download mkcert root CA |

### Headless / Agent Config (no UI)

Drive ngate purely via REST. Set `BASE` to the admin URL (local: `http://localhost:8080`; SSH-tunneled: `http://localhost:8099`).

**Site fields** (`internal/models/models.go`):
`domain`, `proxy_type` (`reverse_proxy`|`static`), `proxy_target`, `static_root`, `certificate_id` (int, optional), `force_https` (bool), `enabled` (bool), `custom_nginx` (string).

**Provider types**: `mkcert`, `letsencrypt_http01`, `letsencrypt_dns01_cloudflare`, `letsencrypt_dns01_route53`. `config` is a YAML string (see "Cert Providers" table above for required keys).

**Recipe 1 — HTTP-only reverse proxy:**
```bash
BASE=http://localhost:8080
curl -X POST $BASE/api/sites -H 'Content-Type: application/json' -d '{
  "domain":"app.example.com",
  "proxy_type":"reverse_proxy",
  "proxy_target":"http://127.0.0.1:3000",
  "enabled":true
}'
```

**Recipe 2 — HTTPS via Let's Encrypt (HTTP-01):**
```bash
# 1. Create provider once
PROV=$(curl -s -X POST $BASE/api/cert-providers -H 'Content-Type: application/json' -d '{
  "name":"le-http",
  "type":"letsencrypt_http01",
  "config":"email: admin@example.com\nstaging: false\n"
}' | jq -r .id)

# 2. Issue cert (async — poll status until "active")
CERT=$(curl -s -X POST $BASE/api/certificates -H 'Content-Type: application/json' -d "{
  \"domain\":\"app.example.com\",
  \"cert_provider_id\":$PROV
}" | jq -r .id)

until [ "$(curl -s $BASE/api/certificates/$CERT | jq -r .status)" = "active" ]; do sleep 3; done

# 3. Create site bound to cert
curl -X POST $BASE/api/sites -H 'Content-Type: application/json' -d "{
  \"domain\":\"app.example.com\",
  \"proxy_type\":\"reverse_proxy\",
  \"proxy_target\":\"http://127.0.0.1:3000\",
  \"certificate_id\":$CERT,
  \"force_https\":true,
  \"enabled\":true
}"
```

**Recipe 3 — Wildcard via Cloudflare DNS-01:**
```bash
curl -X POST $BASE/api/cert-providers -H 'Content-Type: application/json' -d '{
  "name":"cf",
  "type":"letsencrypt_dns01_cloudflare",
  "config":"email: admin@example.com\napi_token: cf-token\n"
}'
# Then POST /api/certificates with "domain":"example.com","alt_domains":"*.example.com"
```

Cert status flow: `pending` → `issuing` → `active` | `error`. Sites with `certificate_id` set will serve HTTPS once the cert is `active`; nginx config regenerates automatically.

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
