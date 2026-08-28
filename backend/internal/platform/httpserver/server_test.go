package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLivez_RegisteredByCaller(t *testing.T) {
	router := NewRouter("http://localhost:5173")
	hc := NewHealthChecker(nil, nil)
	router.Get("/livez", hc.Livez)

	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding response body: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf(`expected {"status":"ok"}, got %v`, body)
	}
}

func TestCORS_PreflightAllowsConfiguredOrigin(t *testing.T) {
	router := NewRouter("http://localhost:5173")
	router.Get("/livez", NewHealthChecker(nil, nil).Livez)

	req := httptest.NewRequest(http.MethodOptions, "/livez", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", "GET")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("expected Access-Control-Allow-Origin http://localhost:5173, got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("expected Access-Control-Allow-Credentials true, got %q", got)
	}
}

func TestCORS_RejectsUnconfiguredOrigin(t *testing.T) {
	router := NewRouter("http://localhost:5173")
	router.Get("/livez", NewHealthChecker(nil, nil).Livez)

	req := httptest.NewRequest(http.MethodOptions, "/livez", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no Access-Control-Allow-Origin for unconfigured origin, got %q", got)
	}
}
