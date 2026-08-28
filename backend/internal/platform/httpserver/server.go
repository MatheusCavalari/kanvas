package httpserver

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/middleware"
)

func NewRouter(allowedOrigin string) chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.CorrelationID, middleware.SlogLogger, middleware.PrometheusHTTP, redactWSToken, chimiddleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{allowedOrigin},
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "X-Correlation-ID"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
	r.Handle("/metrics", promhttp.Handler())
	// Health check routes (/livez, /readyz) are registered by the caller
	// via httpserver.HealthChecker, once pool/redis dependencies exist.
	return r
}
