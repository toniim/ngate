package api

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/ngate/internal/models"
)

// Valid provider types
var validProviderTypes = map[models.ProviderType]bool{
	models.ProviderMkcert:            true,
	models.ProviderLetsEncryptHTTP01: true,
	models.ProviderLetsEncryptDNSR53: true,
	models.ProviderLetsEncryptDNSCF:  true,
}

// --- Cert Provider Handlers ---

func (h *Handler) listCertProviders(c *gin.Context) {
	providers, err := h.db.ListCertProviders()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if providers == nil {
		providers = []models.CertProvider{}
	}
	for i := range providers {
		redactConfig(&providers[i])
	}
	c.JSON(http.StatusOK, providers)
}

func (h *Handler) getCertProvider(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	p, err := h.db.GetCertProvider(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Provider not found"})
		return
	}
	redactConfig(p)
	c.JSON(http.StatusOK, p)
}

func (h *Handler) createCertProvider(c *gin.Context) {
	var p models.CertProvider
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON: " + err.Error()})
		return
	}

	if p.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if !validProviderTypes[p.Type] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid provider type"})
		return
	}
	if normalized, err := normalizeConfigToJSON(p.Config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid config: " + err.Error()})
		return
	} else {
		p.Config = normalized
	}

	if err := h.db.CreateCertProvider(&p); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create: " + err.Error()})
		return
	}

	redactConfig(&p)
	c.JSON(http.StatusOK, p)
}

func (h *Handler) updateCertProvider(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if _, err := h.db.GetCertProvider(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Provider not found"})
		return
	}

	var p models.CertProvider
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}
	p.ID = id

	if p.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if !validProviderTypes[p.Type] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid provider type"})
		return
	}
	if normalized, err := normalizeConfigToJSON(p.Config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid config: " + err.Error()})
		return
	} else {
		p.Config = normalized
	}

	if err := h.db.UpdateCertProvider(&p); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update: " + err.Error()})
		return
	}

	redactConfig(&p)
	c.JSON(http.StatusOK, p)
}

func (h *Handler) deleteCertProvider(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.db.DeleteCertProvider(id); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// --- Certificate Handlers ---

func (h *Handler) listCertificates(c *gin.Context) {
	certs, err := h.db.ListCertificates()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if certs == nil {
		certs = []models.Certificate{}
	}
	c.JSON(http.StatusOK, certs)
}

func (h *Handler) getCertificate(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	cert, err := h.db.GetCertificate(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Certificate not found"})
		return
	}
	c.JSON(http.StatusOK, cert)
}

func (h *Handler) createCertificate(c *gin.Context) {
	var cert models.Certificate
	if err := c.ShouldBindJSON(&cert); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON: " + err.Error()})
		return
	}

	if cert.Domain == "" || !validateDomain(cert.Domain) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid domain"})
		return
	}

	// Validate alt domains if provided
	if cert.AltDomains != "" {
		for _, d := range cert.AltDomainsList() {
			if !validateDomain(d) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid alt domain: " + d})
				return
			}
		}
	}

	provider, err := h.db.GetCertProvider(cert.CertProviderID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cert provider not found"})
		return
	}

	if err := h.db.CreateCertificate(&cert); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create: " + err.Error()})
		return
	}

	go h.issueCertAsync(cert.ID, cert.Domain, cert.AltDomainsList(), provider)

	cert.ProviderType = provider.Type
	c.JSON(http.StatusOK, cert)
}

func (h *Handler) deleteCertificate(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.db.DeleteCertificate(id); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (h *Handler) renewCertificate(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	cert, err := h.db.GetCertificate(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Certificate not found"})
		return
	}

	provider, err := h.db.GetCertProvider(cert.CertProviderID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cert provider not found"})
		return
	}

	go h.issueCertAsync(cert.ID, cert.Domain, cert.AltDomainsList(), provider)
	c.JSON(http.StatusOK, gin.H{"status": "renewing"})
}

// --- Internal ---

func (h *Handler) issueCertAsync(certID int64, domain string, altDomains []string, provider *models.CertProvider) {
	log := logrus.WithFields(logrus.Fields{
		"domain":     domain,
		"altDomains": altDomains,
		"provider":   provider.Type,
	})
	log.Info("Issuing certificate")

	h.db.UpdateCertificateStatus(certID, models.CertStatusIssuing, "", nil)

	err := h.certs.IssueCert(domain, altDomains, provider.Type, provider.Config)
	if err != nil {
		log.WithError(err).Error("Certificate issuance failed")
		h.db.UpdateCertificateStatus(certID, models.CertStatusError, err.Error(), nil)
		return
	}

	expiry, _ := h.certs.GetExpiry(domain, provider.Type)
	h.db.UpdateCertificateStatus(certID, models.CertStatusActive, "", expiry)
	log.Info("Certificate issued successfully")

	// Regenerate nginx config for all sites using this certificate
	sites, _ := h.db.SitesByCertificate(certID)
	for _, site := range sites {
		h.applySiteConfig(&site)
	}
}

func redactConfig(p *models.CertProvider) {
	if p.Config != "" && p.Config != "{}" {
		p.Config = "[REDACTED]"
	}
}

// normalizeConfigToJSON accepts YAML or JSON config string and returns JSON.
// This lets users send YAML (less keystrokes) while we store JSON internally.
func normalizeConfigToJSON(input string) (string, error) {
	if input == "" || input == "{}" {
		return "{}", nil
	}
	var data map[string]interface{}
	if err := yaml.Unmarshal([]byte(input), &data); err != nil {
		return "", err
	}
	out, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
