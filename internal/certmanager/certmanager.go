package certmanager

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/ngate/internal/models"
)

type Manager struct {
	certDir string
}

func New(certDir string) *Manager {
	return &Manager{certDir: certDir}
}

// IssueCert issues a certificate using the given provider type and config.
// altDomains are additional SANs (e.g. *.example.com, api.example.com).
func (m *Manager) IssueCert(domain string, altDomains []string, providerType models.ProviderType, providerConfig string) error {
	switch providerType {
	case models.ProviderMkcert:
		return m.issueMkcert(domain, altDomains)
	case models.ProviderLetsEncryptHTTP01, models.ProviderLetsEncryptDNSR53, models.ProviderLetsEncryptDNSCF:
		return m.issueACME(domain, altDomains, providerType, providerConfig)
	default:
		return fmt.Errorf("unsupported provider type: %s", providerType)
	}
}

// CertPaths returns (certPath, keyPath) for the given provider type and domain
func (m *Manager) CertPaths(domain string, providerType models.ProviderType) (string, string) {
	switch {
	case providerType.IsLetsEncrypt():
		dir := filepath.Join(m.certDir, "letsencrypt", domain)
		return filepath.Join(dir, "fullchain.pem"), filepath.Join(dir, "privkey.pem")
	case providerType == models.ProviderMkcert:
		dir := filepath.Join(m.certDir, "mkcert")
		return filepath.Join(dir, domain+".pem"), filepath.Join(dir, domain+"-key.pem")
	default:
		return "", ""
	}
}

// CertExists checks if certificate files exist on disk
func (m *Manager) CertExists(domain string, providerType models.ProviderType) bool {
	certPath, keyPath := m.CertPaths(domain, providerType)
	if certPath == "" {
		return false
	}
	_, err1 := os.Stat(certPath)
	_, err2 := os.Stat(keyPath)
	return err1 == nil && err2 == nil
}

// GetExpiry reads the certificate and returns expiry time
func (m *Manager) GetExpiry(domain string, providerType models.ProviderType) (*time.Time, error) {
	certPath, _ := m.CertPaths(domain, providerType)
	if certPath == "" {
		return nil, nil
	}

	data, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}

	return &cert.NotAfter, nil
}

func (m *Manager) issueMkcert(domain string, altDomains []string) error {
	certDir := filepath.Join(m.certDir, "mkcert")
	os.MkdirAll(certDir, 0755)

	certFile := filepath.Join(certDir, domain+".pem")
	keyFile := filepath.Join(certDir, domain+"-key.pem")

	// Build args: primary domain + alt domains
	args := []string{"-cert-file", certFile, "-key-file", keyFile, domain}
	args = append(args, altDomains...)

	cmd := exec.Command("mkcert", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mkcert failed: %s - %w", strings.TrimSpace(string(output)), err)
	}

	logrus.WithFields(logrus.Fields{
		"domain":     domain,
		"altDomains": altDomains,
	}).Info("mkcert cert issued")
	return nil
}

// GetMkcertCARoot returns the path to mkcert's root CA
func GetMkcertCARoot() (string, error) {
	cmd := exec.Command("mkcert", "-CAROOT")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("mkcert CAROOT: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}
