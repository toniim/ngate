package models

import (
	"strings"
	"time"
)

// Provider types for certificate issuance
type ProviderType string

const (
	ProviderMkcert              ProviderType = "mkcert"
	ProviderLetsEncryptHTTP01   ProviderType = "letsencrypt_http01"
	ProviderLetsEncryptDNSR53   ProviderType = "letsencrypt_dns01_route53"
	ProviderLetsEncryptDNSCF    ProviderType = "letsencrypt_dns01_cloudflare"
)

func (p ProviderType) IsLetsEncrypt() bool {
	return p == ProviderLetsEncryptHTTP01 || p == ProviderLetsEncryptDNSR53 || p == ProviderLetsEncryptDNSCF
}

// Certificate status
type CertStatus string

const (
	CertStatusPending CertStatus = "pending"
	CertStatusIssuing CertStatus = "issuing"
	CertStatusActive  CertStatus = "active"
	CertStatusError   CertStatus = "error"
	CertStatusExpired CertStatus = "expired"
)

type ProxyType string

const (
	ProxyTypeReverse ProxyType = "reverse_proxy"
	ProxyTypeStatic  ProxyType = "static"
)

// CertProvider holds provider configuration and credentials
type CertProvider struct {
	ID        int64        `json:"id"`
	Name      string       `json:"name"`
	Type      ProviderType `json:"type"`
	Config    string       `json:"config,omitempty"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

// Certificate tracks an issued (or pending) certificate
type Certificate struct {
	ID             int64        `json:"id"`
	Domain         string       `json:"domain"`
	AltDomains     string       `json:"alt_domains,omitempty"` // comma-separated SANs (e.g. "*.example.com,api.example.com")
	CertProviderID int64        `json:"cert_provider_id"`
	ProviderType   ProviderType `json:"provider_type,omitempty"` // populated via JOIN
	Status         CertStatus   `json:"status"`
	ErrorMessage   string       `json:"error_message,omitempty"`
	ExpiresAt      *time.Time   `json:"expires_at,omitempty"`
	IssuedAt       *time.Time   `json:"issued_at,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

// AltDomainsList returns alt_domains as a string slice
func (c *Certificate) AltDomainsList() []string {
	if c.AltDomains == "" {
		return nil
	}
	parts := strings.Split(c.AltDomains, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// AllDomains returns primary domain + alt domains
func (c *Certificate) AllDomains() []string {
	return append([]string{c.Domain}, c.AltDomainsList()...)
}

// Site represents a managed nginx virtual host
type Site struct {
	ID            int64     `json:"id"`
	Domain        string    `json:"domain"`
	ProxyType     ProxyType `json:"proxy_type"`
	ProxyTarget   string    `json:"proxy_target,omitempty"`
	StaticRoot    string    `json:"static_root,omitempty"`
	CertificateID *int64    `json:"certificate_id,omitempty"`
	ForceHTTPS    bool      `json:"force_https"`
	Enabled       bool      `json:"enabled"`
	CustomNginx   string    `json:"custom_nginx,omitempty"`
	NginxError    string    `json:"nginx_error,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type AppConfig struct {
	ListenHTTP   int    `json:"listen_http"`
	ListenHTTPS  int    `json:"listen_https"`
	AdminPort    int    `json:"admin_port"`
	CertDir      string `json:"cert_dir"`
	NginxConfDir string `json:"nginx_conf_dir"`
	DataDir      string `json:"data_dir"`
}
