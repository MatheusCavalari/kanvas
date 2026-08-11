package main

import (
	"context"
	"log"
	"net/http"

	"github.com/MatheusCavalari/kanvas/backend/internal/auth"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/config"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/gen"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/httpserver"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/jwt"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/middleware"
)

func main() {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connecting to database: %v", err)
	}
	defer pool.Close()

	repo := auth.NewPostgresRepository(gen.New(pool))
	issuer := jwt.NewIssuer(cfg.JWTSecret, cfg.AccessTokenTTL)
	service := auth.NewService(repo, issuer, cfg.RefreshTokenTTL)
	handler := auth.NewHandler(service, cfg.SecureCookies)

	router := httpserver.NewRouter()
	handler.RegisterRoutes(router, middleware.Auth(issuer))

	log.Printf("listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, router); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
