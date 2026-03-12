package nginx

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/ngate/internal/models"
)

type Manager struct {
	confDir string
	certDir string
}

func New(confDir, certDir string) *Manager {
	return &Manager{confDir: confDir, certDir: certDir}
}

// SiteConfig holds all data needed to generate an nginx config
type SiteConfig struct {
	Domain      string
	ProxyType   models.ProxyType
	ProxyTarget string
	StaticRoot  string
	CustomNginx string
	ForceHTTPS  bool
	HasSSL      bool
	CertPath    string
	KeyPath     string
}

const siteTemplate = `# Managed by ngate — do not edit manually
# Site: {{.Domain}}

{{if and .ForceHTTPS .HasSSL}}
server {
    listen 80;
    listen [::]:80;
    server_name {{.Domain}};
    return 301 https://$host$request_uri;
}
{{end}}

server {
{{if or (not .ForceHTTPS) (not .HasSSL)}}
    listen 80;
    listen [::]:80;
{{end}}
{{if .HasSSL}}
    listen 443 ssl;
    listen [::]:443 ssl;
    ssl_certificate     {{.CertPath}};
    ssl_certificate_key {{.KeyPath}};
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;
{{end}}

    server_name {{.Domain}};

{{if eq (string .ProxyType) "reverse_proxy"}}
    location / {
        proxy_pass {{.ProxyTarget}};
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_buffering off;
        proxy_read_timeout 86400s;
    }
{{else if eq (string .ProxyType) "static"}}
    root {{.StaticRoot}};
    index index.html index.htm;

    location / {
        try_files $uri $uri/ =404;
    }
{{end}}

{{if .CustomNginx}}
    # Custom directives
    {{.CustomNginx}}
{{end}}

    # ACME challenge for Let's Encrypt
    location /.well-known/acme-challenge/ {
        root /var/www/acme;
        allow all;
    }
}
`

func (m *Manager) GenerateConfig(cfg *SiteConfig) error {
	funcMap := template.FuncMap{
		"string": func(s interface{}) string { return fmt.Sprintf("%v", s) },
	}

	tmpl, err := template.New("site").Funcs(funcMap).Parse(siteTemplate)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	confPath := filepath.Join(m.confDir, fmt.Sprintf("%s.conf", cfg.Domain))
	f, err := os.Create(confPath)
	if err != nil {
		return fmt.Errorf("create config: %w", err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, cfg); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}

	return nil
}

func (m *Manager) RemoveConfig(domain string) error {
	confPath := filepath.Join(m.confDir, fmt.Sprintf("%s.conf", domain))
	return os.Remove(confPath)
}

func (m *Manager) TestConfig() error {
	cmd := exec.Command("nginx", "-t")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nginx test failed: %s - %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

func (m *Manager) Reload() error {
	if err := m.TestConfig(); err != nil {
		return err
	}
	cmd := exec.Command("nginx", "-s", "reload")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nginx reload failed: %s - %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

func (m *Manager) EnsureDirs() error {
	dirs := []string{
		m.confDir,
		filepath.Join(m.certDir, "letsencrypt"),
		filepath.Join(m.certDir, "mkcert"),
		"/var/www/acme",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}
	return nil
}
