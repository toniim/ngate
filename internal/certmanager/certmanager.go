package certmanager

import (
	"bufio"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/ngate/internal/models"
)

// LogFunc is a callback for streaming progress lines during cert issuance.
type LogFunc func(string)

type Manager struct {
	certDir string
}

func New(certDir string) *Manager {
	return &Manager{certDir: certDir}
}

// IssueCert issues a certificate using the given provider type and config.
// altDomains are additional SANs (e.g. *.example.com, api.example.com).
// logFn receives progress lines during issuance (nil-safe: no-op if nil).
func (m *Manager) IssueCert(domain string, altDomains []string, providerType models.ProviderType, providerConfig string, logFn LogFunc) error {
	log := func(format string, args ...interface{}) {
		if logFn != nil {
			logFn(fmt.Sprintf(format, args...))
		}
	}
	switch providerType {
	case models.ProviderMkcert:
		return m.issueMkcert(domain, altDomains, log)
	case models.ProviderLetsEncryptHTTP01, models.ProviderLetsEncryptDNSR53, models.ProviderLetsEncryptDNSCF:
		return m.issueACME(domain, altDomains, providerType, providerConfig, log)
	default:
		return fmt.Errorf("unsupported provider type: %s", providerType)
	}
}

// LogPath returns the on-disk log file path for a given cert ID.
// Caller is responsible for creating parent dir before writing.
func (m *Manager) LogPath(certID int64) string {
	return filepath.Join(m.certDir, "logs", fmt.Sprintf("cert-%d.log", certID))
}

// LogDir returns the directory used for cert log files.
func (m *Manager) LogDir() string {
	return filepath.Join(m.certDir, "logs")
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

func (m *Manager) issueMkcert(domain string, altDomains []string, log func(string, ...interface{})) error {
	certDir := filepath.Join(m.certDir, "mkcert")
	os.MkdirAll(certDir, 0755)

	certFile := filepath.Join(certDir, domain+".pem")
	keyFile := filepath.Join(certDir, domain+"-key.pem")

	args := []string{"-cert-file", certFile, "-key-file", keyFile, domain}
	args = append(args, altDomains...)

	log("Running mkcert for %s...", domain)
	cmd := exec.Command("mkcert", args...)

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("mkcert failed to start: %w", err)
	}

	// Stream stdout and stderr line-by-line
	scanner := bufio.NewScanner(io.MultiReader(stdout, stderr))
	for scanner.Scan() {
		log("%s", scanner.Text())
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("mkcert failed: %w", err)
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
