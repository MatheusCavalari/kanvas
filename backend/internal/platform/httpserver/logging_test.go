package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRedactWSToken verifies that redactWSToken strips a `token` query
// parameter from r.RequestURI before the request reaches the next
// handler in the chain. chimiddleware.Logger reads r.RequestURI (not
// r.URL) when it builds its log line, so this is what actually keeps a
// WebSocket handshake's bearer token out of stdout/log aggregators.
func TestRedactWSToken(t *testing.T) {
	const secret = "secret123"

	var gotRequestURI string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequestURI = r.RequestURI
		w.WriteHeader(http.StatusOK)
	})

	handler := redactWSToken(inner)

	req := httptest.NewRequest(http.MethodGet, "/boards/abc-123/ws?token="+secret, nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if strings.Contains(gotRequestURI, secret) {
		t.Fatalf("expected token to be redacted from RequestURI, got %q", gotRequestURI)
	}
	if !strings.Contains(gotRequestURI, "token=REDACTED") {
		t.Fatalf("expected RequestURI to contain redacted token marker, got %q", gotRequestURI)
	}
}

// TestRedactWSToken_NoQuery verifies requests without a query string
// (the common case for every other route) pass through untouched.
func TestRedactWSToken_NoQuery(t *testing.T) {
	var gotRequestURI string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequestURI = r.RequestURI
		w.WriteHeader(http.StatusOK)
	})

	handler := redactWSToken(inner)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if gotRequestURI != "/healthz" {
		t.Fatalf("expected RequestURI to be unchanged, got %q", gotRequestURI)
	}
}

// TestRedactWSToken_OtherParamsPreserved verifies non-token query
// parameters survive redaction unchanged.
func TestRedactWSToken_OtherParamsPreserved(t *testing.T) {
	var gotRequestURI string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequestURI = r.RequestURI
		w.WriteHeader(http.StatusOK)
	})

	handler := redactWSToken(inner)

	req := httptest.NewRequest(http.MethodGet, "/boards/abc-123/ws?foo=bar&token=secret123", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !strings.Contains(gotRequestURI, "foo=bar") {
		t.Fatalf("expected non-token param to be preserved, got %q", gotRequestURI)
	}
	if strings.Contains(gotRequestURI, "secret123") {
		t.Fatalf("expected token to be redacted, got %q", gotRequestURI)
	}
}
