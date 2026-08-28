package httpserver

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

func NewRouter(allowedOrigin string) chi.Router {
	r := chi.NewRouter()
	r.Use(redactWSToken, chimiddleware.Logger, chimiddleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{allowedOrigin},
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
	// Health check routes (/livez, /readyz) are registered by the caller
	// via httpserver.HealthChecker, once pool/redis dependencies exist.
	return r
}
