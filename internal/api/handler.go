package api

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/ngate/internal/certmanager"
	"github.com/ngate/internal/db"
	"github.com/ngate/internal/models"
	"github.com/ngate/internal/nginx"
)

// domainRe validates domain names (alphanumeric, hyphens, dots, wildcards)
var domainRe = regexp.MustCompile(`^(\*\.)?([a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?\.)*[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$`)

type Handler struct {
	db    *db.DB
	nginx *nginx.Manager
	certs *certmanager.Manager
}

func NewHandler(db *db.DB, nginx *nginx.Manager, certs *certmanager.Manager) *Handler {
	return &Handler{db: db, nginx: nginx, certs: certs}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	// Sites
	rg.GET("/sites", h.listSites)
	rg.POST("/sites", h.createSite)
	rg.GET("/sites/:id", h.getSite)
	rg.PUT("/sites/:id", h.updateSite)
	rg.DELETE("/sites/:id", h.deleteSite)
	rg.POST("/sites/:id/enable", h.toggleSite)

	// Cert Providers
	rg.GET("/cert-providers", h.listCertProviders)
	rg.POST("/cert-providers", h.createCertProvider)
	rg.GET("/cert-providers/:id", h.getCertProvider)
	rg.PUT("/cert-providers/:id", h.updateCertProvider)
	rg.DELETE("/cert-providers/:id", h.deleteCertProvider)

	// Certificates
	rg.GET("/certificates", h.listCertificates)
	rg.POST("/certificates", h.createCertificate)
	rg.GET("/certificates/:id", h.getCertificate)
	rg.DELETE("/certificates/:id", h.deleteCertificate)
	rg.POST("/certificates/:id/renew", h.renewCertificate)

	// Nginx
	rg.GET("/nginx/status", h.nginxStatus)
	rg.POST("/nginx/reload", h.nginxReload)
	rg.GET("/mkcert/caroot", h.mkcertCARoot)
	rg.GET("/mkcert/rootca.pem", h.mkcertDownloadCA)
}

// --- Site Handlers ---

func (h *Handler) listSites(c *gin.Context) {
	sites, err := h.db.ListSites()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if sites == nil {
		sites = []models.Site{}
	}
	c.JSON(http.StatusOK, sites)
}

func (h *Handler) getSite(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	site, err := h.db.GetSite(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Site not found"})
		return
	}
	c.JSON(http.StatusOK, site)
}

func (h *Handler) createSite(c *gin.Context) {
	var site models.Site
	if err := c.ShouldBindJSON(&site); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON: " + err.Error()})
		return
	}

	if site.Domain == "" || !validateDomain(site.Domain) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid domain"})
		return
	}
	if site.ProxyType == "" {
		site.ProxyType = models.ProxyTypeReverse
	}
	site.CustomNginx = sanitizeNginxDirectives(site.CustomNginx)

	if err := h.db.CreateSite(&site); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create: " + err.Error()})
		return
	}

	if site.Enabled {
		h.applySiteConfig(&site)
	}

	c.JSON(http.StatusOK, site)
}

func (h *Handler) updateSite(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	existing, err := h.db.GetSite(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Site not found"})
		return
	}

	var site models.Site
	if err := c.ShouldBindJSON(&site); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}
	site.ID = id

	if site.Domain != "" && !validateDomain(site.Domain) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid domain"})
		return
	}
	site.CustomNginx = sanitizeNginxDirectives(site.CustomNginx)

	// Remove old config if domain changed
	if existing.Domain != site.Domain {
		h.nginx.RemoveConfig(existing.Domain)
	}

	if err := h.db.UpdateSite(&site); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update: " + err.Error()})
		return
	}

	if site.Enabled {
		h.applySiteConfig(&site)
	} else {
		h.nginx.RemoveConfig(site.Domain)
		h.nginx.Reload()
	}

	c.JSON(http.StatusOK, site)
}

func (h *Handler) deleteSite(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	site, err := h.db.GetSite(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Site not found"})
		return
	}

	h.nginx.RemoveConfig(site.Domain)
	h.db.DeleteSite(id)
	h.nginx.Reload()

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (h *Handler) toggleSite(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	site, err := h.db.GetSite(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Site not found"})
		return
	}

	site.Enabled = !site.Enabled
	h.db.UpdateSite(site)

	if site.Enabled {
		h.applySiteConfig(site)
	} else {
		h.nginx.RemoveConfig(site.Domain)
		h.nginx.Reload()
	}

	c.JSON(http.StatusOK, site)
}

// --- Nginx Handlers ---

func (h *Handler) nginxStatus(c *gin.Context) {
	err := h.nginx.TestConfig()
	status := "ok"
	msg := "Configuration valid"
	if err != nil {
		status = "error"
		msg = err.Error()
	}
	c.JSON(http.StatusOK, gin.H{"status": status, "message": msg})
}

func (h *Handler) nginxReload(c *gin.Context) {
	if err := h.nginx.Reload(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "reloaded"})
}

func (h *Handler) mkcertCARoot(c *gin.Context) {
	root, err := certmanager.GetMkcertCARoot()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "mkcert not installed or CAROOT not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"caroot":        root,
		"trust_macos":   "sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain " + root + "/rootCA.pem",
		"trust_windows": `certutil -addstore -f "ROOT" ` + root + `\rootCA.pem`,
		"trust_linux":   "sudo cp " + root + "/rootCA.pem /usr/local/share/ca-certificates/mkcert-ca.crt && sudo update-ca-certificates",
	})
}

func (h *Handler) mkcertDownloadCA(c *gin.Context) {
	root, err := certmanager.GetMkcertCARoot()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "mkcert not installed or CAROOT not found"})
		return
	}
	caFile := root + "/rootCA.pem"
	c.Header("Content-Disposition", "attachment; filename=rootCA.pem")
	c.File(caFile)
}

// --- Internal ---

func (h *Handler) applySiteConfig(site *models.Site) {
	cfg := &nginx.SiteConfig{
		Domain:      site.Domain,
		ProxyType:   site.ProxyType,
		ProxyTarget: site.ProxyTarget,
		StaticRoot:  site.StaticRoot,
		CustomNginx: site.CustomNginx,
		ForceHTTPS:  site.ForceHTTPS,
	}

	// Resolve certificate paths if site has a cert assigned
	if site.CertificateID != nil {
		cert, err := h.db.GetCertificate(*site.CertificateID)
		if err == nil && cert.Status == models.CertStatusActive {
			certPath, keyPath := h.certs.CertPaths(cert.Domain, cert.ProviderType)
			if h.certs.CertExists(cert.Domain, cert.ProviderType) {
				cfg.HasSSL = true
				cfg.CertPath = certPath
				cfg.KeyPath = keyPath
			}
		}
	}

	if err := h.nginx.GenerateConfig(cfg); err != nil {
		logrus.WithFields(logrus.Fields{
			"domain": site.Domain,
			"error":  err,
		}).Error("Failed to generate nginx config")
		return
	}
	if err := h.nginx.Reload(); err != nil {
		logrus.WithFields(logrus.Fields{
			"domain": site.Domain,
			"error":  err,
		}).Error("Failed to reload nginx")
	}
}

// --- Helpers ---

// parseID extracts and validates the :id URL param
func parseID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return 0, false
	}
	return id, true
}

// validateDomain checks for valid domain name to prevent path traversal and injection
func validateDomain(domain string) bool {
	if len(domain) == 0 || len(domain) > 253 {
		return false
	}
	return domainRe.MatchString(domain)
}

// sanitizeNginxDirectives removes dangerous directives from custom nginx config
func sanitizeNginxDirectives(input string) string {
	dangerous := []string{"lua_", "access_by_lua", "content_by_lua", "rewrite_by_lua",
		"load_module", "include ", "ssl_certificate", "ssl_certificate_key"}
	lines := strings.Split(input, "\n")
	var safe []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(strings.ToLower(line))
		blocked := false
		for _, d := range dangerous {
			if strings.HasPrefix(trimmed, d) {
				blocked = true
				break
			}
		}
		if !blocked {
			safe = append(safe, line)
		}
	}
	return strings.Join(safe, "\n")
}
