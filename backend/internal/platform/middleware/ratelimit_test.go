package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/middleware"
)

func TestIPKey(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	key := middleware.IPKey(req)
	require.Equal(t, "ip:192.168.1.1", key)
}

func TestIPKey_NoPort(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.1"
	key := middleware.IPKey(req)
	require.Equal(t, "ip:192.168.1.1", key)
}

func TestUserKey_NoUser(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	key := middleware.UserKey(req)
	require.Empty(t, key)
}

func TestUserWriteKey_GetIsSkipped(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	key := middleware.UserWriteKey(req)
	require.Empty(t, key)
}

func TestUserWriteKey_OptionsIsSkipped(t *testing.T) {
	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	key := middleware.UserWriteKey(req)
	require.Empty(t, key)
}

func TestUserWriteKey_PostNoUser(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	key := middleware.UserWriteKey(req)
	require.Empty(t, key)
}

func TestUserReadKey_PostIsSkipped(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	key := middleware.UserReadKey(req)
	require.Empty(t, key)
}

func TestUserReadKey_GetNoUser(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	key := middleware.UserReadKey(req)
	require.Empty(t, key)
}

func TestNewRateLimiter_EmptyKeySkipsLimiting(t *testing.T) {
	// A nil *redis.Client is fine here because keyFn always returns "",
	// so the middleware must never touch the client.
	rl := middleware.NewRateLimiter(nil, 1, 0, func(r *http.Request) string {
		return ""
	})

	called := false
	handler := rl(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.True(t, called)
	require.Equal(t, http.StatusOK, rec.Code)
}
