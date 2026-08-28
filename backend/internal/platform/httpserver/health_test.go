package httpserver_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/httpserver"
)

func TestLivez_ReturnsOK(t *testing.T) {
	hc := httpserver.NewHealthChecker(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	rec := httptest.NewRecorder()
	hc.Livez(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"ok"`)
}

func TestLivez_ShuttingDown(t *testing.T) {
	hc := httpserver.NewHealthChecker(nil, nil)
	hc.SetShuttingDown()
	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	rec := httptest.NewRecorder()
	hc.Livez(rec, req)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Contains(t, rec.Body.String(), `"shutting_down"`)
}

func TestReadyz_NilDependencies_NotConfigured(t *testing.T) {
	hc := httpserver.NewHealthChecker(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	hc.Readyz(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"not_configured"`)
	require.Contains(t, rec.Body.String(), `"ready"`)
}
