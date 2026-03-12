package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/ngate/internal/models"
)

// --- Cert Providers ---

func (db *DB) ListCertProviders() ([]models.CertProvider, error) {
	rows, err := db.conn.Query(`
		SELECT id, name, type, config, created_at, updated_at
		FROM cert_providers ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var providers []models.CertProvider
	for rows.Next() {
		var p models.CertProvider
		if err := rows.Scan(&p.ID, &p.Name, &p.Type, &p.Config, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		providers = append(providers, p)
	}
	return providers, nil
}

func (db *DB) GetCertProvider(id int64) (*models.CertProvider, error) {
	var p models.CertProvider
	err := db.conn.QueryRow(`
		SELECT id, name, type, config, created_at, updated_at
		FROM cert_providers WHERE id = ?
	`, id).Scan(&p.ID, &p.Name, &p.Type, &p.Config, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (db *DB) CreateCertProvider(p *models.CertProvider) error {
	now := time.Now()
	if p.Config == "" {
		p.Config = "{}"
	}
	result, err := db.conn.Exec(`
		INSERT INTO cert_providers (name, type, config, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, p.Name, p.Type, p.Config, now, now)
	if err != nil {
		return err
	}
	p.ID, _ = result.LastInsertId()
	p.CreatedAt = now
	p.UpdatedAt = now
	return nil
}

func (db *DB) UpdateCertProvider(p *models.CertProvider) error {
	now := time.Now()
	_, err := db.conn.Exec(`
		UPDATE cert_providers SET name=?, type=?, config=?, updated_at=?
		WHERE id=?
	`, p.Name, p.Type, p.Config, now, p.ID)
	p.UpdatedAt = now
	return err
}

func (db *DB) DeleteCertProvider(id int64) error {
	var count int
	db.conn.QueryRow("SELECT COUNT(*) FROM certificates WHERE cert_provider_id=?", id).Scan(&count)
	if count > 0 {
		return fmt.Errorf("provider has %d certificate(s); delete them first", count)
	}
	_, err := db.conn.Exec("DELETE FROM cert_providers WHERE id=?", id)
	return err
}

// --- Certificates ---

func (db *DB) ListCertificates() ([]models.Certificate, error) {
	rows, err := db.conn.Query(`
		SELECT c.id, c.domain, c.alt_domains, c.cert_provider_id, p.type,
		       c.status, c.error_message, c.expires_at, c.issued_at,
		       c.created_at, c.updated_at
		FROM certificates c
		JOIN cert_providers p ON p.id = c.cert_provider_id
		ORDER BY c.domain
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var certs []models.Certificate
	for rows.Next() {
		c, err := scanCert(rows)
		if err != nil {
			return nil, err
		}
		certs = append(certs, c)
	}
	return certs, nil
}

func (db *DB) GetCertificate(id int64) (*models.Certificate, error) {
	rows, err := db.conn.Query(`
		SELECT c.id, c.domain, c.alt_domains, c.cert_provider_id, p.type,
		       c.status, c.error_message, c.expires_at, c.issued_at,
		       c.created_at, c.updated_at
		FROM certificates c
		JOIN cert_providers p ON p.id = c.cert_provider_id
		WHERE c.id = ?
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, sql.ErrNoRows
	}
	c, err := scanCert(rows)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (db *DB) CreateCertificate(c *models.Certificate) error {
	now := time.Now()
	result, err := db.conn.Exec(`
		INSERT INTO certificates (domain, alt_domains, cert_provider_id, status, created_at, updated_at)
		VALUES (?, ?, ?, 'pending', ?, ?)
	`, c.Domain, c.AltDomains, c.CertProviderID, now, now)
	if err != nil {
		return err
	}
	c.ID, _ = result.LastInsertId()
	c.Status = models.CertStatusPending
	c.CreatedAt = now
	c.UpdatedAt = now
	return nil
}

func (db *DB) UpdateCertificateStatus(id int64, status models.CertStatus, errMsg string, expiresAt *time.Time) error {
	now := time.Now()
	var issuedAt *time.Time
	if status == models.CertStatusActive {
		issuedAt = &now
	}
	_, err := db.conn.Exec(`
		UPDATE certificates SET status=?, error_message=?, expires_at=?, issued_at=?, updated_at=?
		WHERE id=?
	`, status, errMsg, expiresAt, issuedAt, now, id)
	return err
}

func (db *DB) DeleteCertificate(id int64) error {
	var count int
	db.conn.QueryRow("SELECT COUNT(*) FROM sites WHERE certificate_id=?", id).Scan(&count)
	if count > 0 {
		return fmt.Errorf("certificate is used by %d site(s); unlink them first", count)
	}
	_, err := db.conn.Exec("DELETE FROM certificates WHERE id=?", id)
	return err
}

// SitesByCertificate returns all enabled sites using a given certificate
func (db *DB) SitesByCertificate(certID int64) ([]models.Site, error) {
	rows, err := db.conn.Query(`
		SELECT id, domain, proxy_type, proxy_target, static_root,
		       certificate_id, force_https, enabled, custom_nginx,
		       created_at, updated_at
		FROM sites WHERE certificate_id = ? AND enabled = 1
	`, certID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sites []models.Site
	for rows.Next() {
		s, err := scanSite(rows)
		if err != nil {
			return nil, err
		}
		sites = append(sites, s)
	}
	return sites, nil
}

func scanCert(rows *sql.Rows) (models.Certificate, error) {
	var c models.Certificate
	var expiresAt, issuedAt sql.NullTime
	err := rows.Scan(
		&c.ID, &c.Domain, &c.AltDomains, &c.CertProviderID, &c.ProviderType,
		&c.Status, &c.ErrorMessage, &expiresAt, &issuedAt,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return c, err
	}
	if expiresAt.Valid {
		c.ExpiresAt = &expiresAt.Time
	}
	if issuedAt.Valid {
		c.IssuedAt = &issuedAt.Time
	}
	return c, nil
}
