package middleware_test

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/middleware"
)

func TestCorrelationID_GeneratesWhenMissing(t *testing.T) {
	handler := middleware.CorrelationID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cid := middleware.CorrelationIDFromContext(r.Context())
		require.NotEmpty(t, cid)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.NotEmpty(t, rec.Header().Get("X-Correlation-ID"))
}

func TestCorrelationID_PreservesExisting(t *testing.T) {
	handler := middleware.CorrelationID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cid := middleware.CorrelationIDFromContext(r.Context())
		require.Equal(t, "test-123", cid)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Correlation-ID", "test-123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, "test-123", rec.Header().Get("X-Correlation-ID"))
}

func TestCorrelationIDFromContext_EmptyWhenAbsent(t *testing.T) {
	require.Empty(t, middleware.CorrelationIDFromContext(t.Context()))
}

func TestSlogLogger_CallsNextAndPreservesStatus(t *testing.T) {
	called := false
	handler := middleware.SlogLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.True(t, called)
	require.Equal(t, http.StatusTeapot, rec.Code)
}

// hijackableRecorder is a ResponseWriter that also implements
// http.Hijacker, standing in for the writer chi's WebSocket-friendly
// server produces in production.
type hijackableRecorder struct {
	*httptest.ResponseRecorder
	hijacked bool
}

func (h *hijackableRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h.hijacked = true
	server, _ := net.Pipe()
	return server, bufio.NewReadWriter(bufio.NewReader(server), bufio.NewWriter(server)), nil
}

func TestSlogLogger_PreservesHijackerForWebSocketUpgrades(t *testing.T) {
	var hijackErr error
	handler := middleware.SlogLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		require.True(t, ok, "response writer wrapped by SlogLogger must still implement http.Hijacker")
		_, _, hijackErr = hijacker.Hijack()
	}))

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	rec := &hijackableRecorder{ResponseRecorder: httptest.NewRecorder()}
	handler.ServeHTTP(rec, req)

	require.NoError(t, hijackErr)
	require.True(t, rec.hijacked)
}
