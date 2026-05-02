package certmanager

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/providers/http/webroot"
	"github.com/go-acme/lego/v4/registration"

	"github.com/ngate/internal/models"
)

const (
	leProductionCA = lego.LEDirectoryProduction
	leStagingCA    = lego.LEDirectoryStaging
	webrootPath    = "/var/www/acme"
)

// acmeUser implements registration.User for lego
type acmeUser struct {
	Email        string                 `json:"email"`
	Registration *registration.Resource `json:"registration"`
	key          crypto.PrivateKey
}

func (u *acmeUser) GetEmail() string                        { return u.Email }
func (u *acmeUser) GetRegistration() *registration.Resource { return u.Registration }
func (u *acmeUser) GetPrivateKey() crypto.PrivateKey        { return u.key }

// acmeConfig is the common config fields parsed from provider JSON
type acmeConfig struct {
	Email   string `json:"email"`
	Staging bool   `json:"staging"`
}

func (m *Manager) issueACME(domain string, altDomains []string, providerType models.ProviderType, configJSON string, log func(string, ...interface{})) error {
	var cfg acmeConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return fmt.Errorf("parse provider config: %w", err)
	}
	if cfg.Email == "" {
		cfg.Email = "admin@" + domain
	}

	caLabel := "production"
	if cfg.Staging {
		caLabel = "staging"
	}
	log("Loading ACME account for %s (%s)...", cfg.Email, caLabel)
	user, err := m.loadOrCreateAccount(cfg.Email, caLabel)
	if err != nil {
		return fmt.Errorf("acme account: %w", err)
	}

	legoConfig := lego.NewConfig(user)
	legoConfig.Certificate.KeyType = certcrypto.RSA2048
	if cfg.Staging {
		legoConfig.CADirURL = leStagingCA
	} else {
		legoConfig.CADirURL = leProductionCA
	}

	client, err := lego.NewClient(legoConfig)
	if err != nil {
		return fmt.Errorf("lego client: %w", err)
	}

	// Register account if needed
	if user.Registration == nil {
		log("Registering new account with Let's Encrypt (%s)...", caLabel)
		reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
		if err != nil {
			return fmt.Errorf("acme register: %w", err)
		}
		user.Registration = reg
		m.saveAccount(user, caLabel)
	}

	// Set up challenge provider
	if providerType == models.ProviderLetsEncryptHTTP01 {
		log("Setting up HTTP-01 challenge provider (webroot: %s)...", webrootPath)
		provider, err := webroot.NewHTTPProvider(webrootPath)
		if err != nil {
			return fmt.Errorf("webroot provider: %w", err)
		}
		client.Challenge.SetHTTP01Provider(provider)
	} else {
		log("Setting up DNS-01 challenge provider...")
		dnsProvider, err := buildDNSProvider(providerType, configJSON)
		if err != nil {
			return fmt.Errorf("dns provider: %w", err)
		}
		client.Challenge.SetDNS01Provider(dnsProvider)
	}

	// Obtain certificate with all domains (primary + SANs)
	allDomains := append([]string{domain}, altDomains...)
	log("Requesting certificate from Let's Encrypt for: %s...", strings.Join(allDomains, ", "))
	request := certificate.ObtainRequest{
		Domains: allDomains,
		Bundle:  true,
	}
	certificates, err := client.Certificate.Obtain(request)
	if err != nil {
		return fmt.Errorf("obtain cert: %w", err)
	}

	// Write cert files
	certDir := filepath.Join(m.certDir, "letsencrypt", domain)
	os.MkdirAll(certDir, 0755)

	log("Writing certificate files to %s...", certDir)
	if err := os.WriteFile(filepath.Join(certDir, "fullchain.pem"), certificates.Certificate, 0644); err != nil {
		return fmt.Errorf("write cert: %w", err)
	}
	if err := os.WriteFile(filepath.Join(certDir, "privkey.pem"), certificates.PrivateKey, 0600); err != nil {
		return fmt.Errorf("write key: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"domain":   domain,
		"provider": providerType,
		"staging":  cfg.Staging,
	}).Info("ACME cert issued")
	return nil
}

// loadOrCreateAccount loads or creates an ACME account for the given email
func (m *Manager) loadOrCreateAccount(email, caLabel string) (*acmeUser, error) {
	accountDir := filepath.Join(m.certDir, "accounts", caLabel, email)
	keyPath := filepath.Join(accountDir, "account-key.pem")
	regPath := filepath.Join(accountDir, "registration.json")

	user := &acmeUser{Email: email}

	// Try load existing
	if keyData, err := os.ReadFile(keyPath); err == nil {
		block, _ := pem.Decode(keyData)
		if block != nil {
			key, err := x509.ParseECPrivateKey(block.Bytes)
			if err == nil {
				user.key = key
				if regData, err := os.ReadFile(regPath); err == nil {
					json.Unmarshal(regData, user)
				}
				return user, nil
			}
		}
	}

	// Generate new key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}
	user.key = privateKey

	// Persist key
	os.MkdirAll(accountDir, 0700)
	keyBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("marshal key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return nil, fmt.Errorf("write key: %w", err)
	}

	return user, nil
}

func (m *Manager) saveAccount(user *acmeUser, caLabel string) {
	accountDir := filepath.Join(m.certDir, "accounts", caLabel, user.Email)
	regPath := filepath.Join(accountDir, "registration.json")
	data, _ := json.Marshal(user)
	os.WriteFile(regPath, data, 0600)
}
