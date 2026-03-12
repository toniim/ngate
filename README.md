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

## Quick Start (Local / Ubuntu)

```bash
# Prerequisites
sudo apt install nginx certbot

# Install mkcert (optional, for local SSL)
curl -JLO "https://dl.filippo.io/mkcert/latest?for=linux/amd64"
chmod +x mkcert-v*-linux-amd64
sudo mv mkcert-v*-linux-amd64 /usr/local/bin/mkcert
mkcert -install

# Build & run
make build
sudo make run
```

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
