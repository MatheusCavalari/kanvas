package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/metrics"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/middleware"
)

func TestPrometheusHTTP_RecordsRequestCountByRoutePattern(t *testing.T) {
	router := chi.NewRouter()
	router.Use(middleware.PrometheusHTTP)
	router.Get("/widgets/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	before := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("GET", "/widgets/{id}", "201"))

	req := httptest.NewRequest(http.MethodGet, "/widgets/42", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)

	after := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("GET", "/widgets/{id}", "201"))
	require.Equal(t, before+1, after, "expected HTTPRequestsTotal to be incremented for the route pattern, not the raw path")
}

func TestPrometheusHTTP_FallsBackToRawPathWhenNoRoutePattern(t *testing.T) {
	// A bare handler with no chi routing context set means RoutePattern()
	// returns "", so the middleware should fall back to r.URL.Path.
	handler := middleware.PrometheusHTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	before := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("GET", "/no-route", "200"))

	req := httptest.NewRequest(http.MethodGet, "/no-route", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	after := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("GET", "/no-route", "200"))
	require.Equal(t, before+1, after)
}

func TestPrometheusHTTP_ObservesRequestDuration(t *testing.T) {
	handler := middleware.PrometheusHTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	before := testutil.CollectAndCount(metrics.HTTPRequestDuration)

	req := httptest.NewRequest(http.MethodGet, "/duration-check", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	after := testutil.CollectAndCount(metrics.HTTPRequestDuration)
	require.Greater(t, after, before, "expected a new duration observation to add a new label combination")
}
