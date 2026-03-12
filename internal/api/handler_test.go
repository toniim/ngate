package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ngate/internal/certmanager"
	"github.com/ngate/internal/db"
	"github.com/ngate/internal/nginx"
)

func setupTestRouter(t *testing.T) (*gin.Engine, *Handler) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	database, err := db.New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	tmpDir := t.TempDir()
	nginxMgr := nginx.New(tmpDir+"/conf", tmpDir+"/certs")
	certMgr := certmanager.New(tmpDir + "/certs")
	handler := NewHandler(database, nginxMgr, certMgr)

	r := gin.New()
	handler.RegisterRoutes(r.Group("/api"))
	return r, handler
}

func TestListSitesEmpty(t *testing.T) {
	r, _ := setupTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/sites", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var sites []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &sites); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(sites) != 0 {
		t.Fatalf("expected empty list, got %d", len(sites))
	}
}

func TestCreateSite(t *testing.T) {
	r, _ := setupTestRouter(t)

	body := `{"domain":"example.com","proxy_type":"reverse_proxy","proxy_target":"http://localhost:3000","enabled":false}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/sites", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var site map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &site)
	if site["domain"] != "example.com" {
		t.Fatalf("expected domain example.com, got %v", site["domain"])
	}
}

func TestCreateSiteInvalidDomain(t *testing.T) {
	r, _ := setupTestRouter(t)

	body := `{"domain":"../etc/passwd","proxy_target":"http://localhost:3000"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/sites", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetSiteNotFound(t *testing.T) {
	r, _ := setupTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/sites/999", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestInvalidID(t *testing.T) {
	r, _ := setupTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/sites/abc", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCRUDSiteFlow(t *testing.T) {
	r, _ := setupTestRouter(t)

	// Create
	body := `{"domain":"test.dev","proxy_type":"reverse_proxy","proxy_target":"http://localhost:8080","enabled":false}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/sites", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("create: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var created map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &created)
	id := created["id"].(float64)

	// Get
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/sites/1", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", w.Code)
	}

	// List
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/sites", nil)
	r.ServeHTTP(w, req)
	var sites []map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &sites)
	if len(sites) != 1 {
		t.Fatalf("list: expected 1 site, got %d", len(sites))
	}

	// Delete
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("DELETE", "/api/sites/1", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d", w.Code)
	}

	_ = id // used implicitly via URL
}

func TestListCertProvidersEmpty(t *testing.T) {
	r, _ := setupTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/cert-providers", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestNginxStatusEndpoint(t *testing.T) {
	r, _ := setupTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/nginx/status", nil)
	r.ServeHTTP(w, req)

	// nginx not installed in test env, but endpoint should still respond
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestValidateDomain(t *testing.T) {
	tests := []struct {
		domain string
		valid  bool
	}{
		{"example.com", true},
		{"sub.example.com", true},
		{"*.example.com", true},
		{"a-b.com", true},
		{"", false},
		{"../etc", false},
		{"-bad.com", false},
	}
	for _, tt := range tests {
		if got := validateDomain(tt.domain); got != tt.valid {
			t.Errorf("validateDomain(%q) = %v, want %v", tt.domain, got, tt.valid)
		}
	}
}

func TestSanitizeNginxDirectives(t *testing.T) {
	input := "proxy_pass http://backend;\nlua_code_cache on;\ninclude /etc/secret;"
	result := sanitizeNginxDirectives(input)
	if strings.Contains(result, "lua_") {
		t.Error("lua directive not sanitized")
	}
	if strings.Contains(result, "include ") {
		t.Error("include directive not sanitized")
	}
	if !strings.Contains(result, "proxy_pass") {
		t.Error("proxy_pass should be kept")
	}
}
