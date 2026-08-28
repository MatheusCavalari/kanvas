package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/metrics"
)

// PrometheusHTTP is HTTP middleware that records request counts and
// latency histograms in Prometheus, labeled by method, chi route pattern,
// and (for the counter) response status code.
func PrometheusHTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		duration := time.Since(start).Seconds()

		// RoutePattern() is only populated once routing has completed,
		// which is why we read it after next.ServeHTTP has run.
		routePattern := chi.RouteContext(r.Context()).RoutePattern()
		if routePattern == "" {
			routePattern = r.URL.Path
		}

		metrics.HTTPRequestsTotal.WithLabelValues(r.Method, routePattern, strconv.Itoa(rec.status)).Inc()
		metrics.HTTPRequestDuration.WithLabelValues(r.Method, routePattern).Observe(duration)
	})
}
