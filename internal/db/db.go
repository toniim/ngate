package db

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/ngate/internal/models"
)

type DB struct {
	conn *sql.DB
}

func New(path string) (*DB, error) {
	conn, err := sql.Open("sqlite3", path+"?_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS cert_providers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			config TEXT DEFAULT '{}',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS certificates (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			domain TEXT NOT NULL,
			cert_provider_id INTEGER NOT NULL REFERENCES cert_providers(id),
			status TEXT NOT NULL DEFAULT 'pending',
			error_message TEXT DEFAULT '',
			expires_at DATETIME,
			issued_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS sites (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			domain TEXT NOT NULL UNIQUE,
			proxy_type TEXT NOT NULL DEFAULT 'reverse_proxy',
			proxy_target TEXT DEFAULT '',
			static_root TEXT DEFAULT '',
			certificate_id INTEGER REFERENCES certificates(id),
			force_https INTEGER DEFAULT 0,
			enabled INTEGER DEFAULT 1,
			custom_nginx TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}
	for _, m := range migrations {
		if _, err := db.conn.Exec(m); err != nil {
			return err
		}
	}

	// Column migrations (ignore errors if already exists)
	db.conn.Exec(`ALTER TABLE certificates ADD COLUMN alt_domains TEXT DEFAULT ''`)

	return nil
}

func (db *DB) ListSites() ([]models.Site, error) {
	rows, err := db.conn.Query(`
		SELECT id, domain, proxy_type, proxy_target, static_root,
		       certificate_id, force_https, enabled, custom_nginx,
		       created_at, updated_at
		FROM sites ORDER BY domain
	`)
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

func (db *DB) GetSite(id int64) (*models.Site, error) {
	row := db.conn.QueryRow(`
		SELECT id, domain, proxy_type, proxy_target, static_root,
		       certificate_id, force_https, enabled, custom_nginx,
		       created_at, updated_at
		FROM sites WHERE id = ?
	`, id)
	s, err := scanSiteRow(row)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (db *DB) CreateSite(s *models.Site) error {
	now := time.Now()
	result, err := db.conn.Exec(`
		INSERT INTO sites (domain, proxy_type, proxy_target, static_root,
		                    certificate_id, force_https, enabled, custom_nginx,
		                    created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, s.Domain, s.ProxyType, s.ProxyTarget, s.StaticRoot,
		s.CertificateID, boolToInt(s.ForceHTTPS), boolToInt(s.Enabled), s.CustomNginx,
		now, now,
	)
	if err != nil {
		return err
	}
	s.ID, _ = result.LastInsertId()
	s.CreatedAt = now
	s.UpdatedAt = now
	return nil
}

func (db *DB) UpdateSite(s *models.Site) error {
	now := time.Now()
	_, err := db.conn.Exec(`
		UPDATE sites SET domain=?, proxy_type=?, proxy_target=?, static_root=?,
		                 certificate_id=?, force_https=?, enabled=?, custom_nginx=?,
		                 updated_at=?
		WHERE id=?
	`, s.Domain, s.ProxyType, s.ProxyTarget, s.StaticRoot,
		s.CertificateID, boolToInt(s.ForceHTTPS), boolToInt(s.Enabled), s.CustomNginx,
		now, s.ID,
	)
	s.UpdatedAt = now
	return err
}

func (db *DB) DeleteSite(id int64) error {
	_, err := db.conn.Exec("DELETE FROM sites WHERE id=?", id)
	return err
}

// scanSite scans a site row from rows.Scan
func scanSite(rows *sql.Rows) (models.Site, error) {
	var s models.Site
	var certID sql.NullInt64
	var forceHTTPS, enabled int
	err := rows.Scan(
		&s.ID, &s.Domain, &s.ProxyType, &s.ProxyTarget, &s.StaticRoot,
		&certID, &forceHTTPS, &enabled, &s.CustomNginx,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return s, err
	}
	s.ForceHTTPS = forceHTTPS == 1
	s.Enabled = enabled == 1
	if certID.Valid {
		s.CertificateID = &certID.Int64
	}
	return s, nil
}

func scanSiteRow(row *sql.Row) (models.Site, error) {
	var s models.Site
	var certID sql.NullInt64
	var forceHTTPS, enabled int
	err := row.Scan(
		&s.ID, &s.Domain, &s.ProxyType, &s.ProxyTarget, &s.StaticRoot,
		&certID, &forceHTTPS, &enabled, &s.CustomNginx,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return s, err
	}
	s.ForceHTTPS = forceHTTPS == 1
	s.Enabled = enabled == 1
	if certID.Valid {
		s.CertificateID = &certID.Int64
	}
	return s, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
