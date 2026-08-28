package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/redis/go-redis/v9"

	"github.com/MatheusCavalari/kanvas/backend/internal/activity"
	"github.com/MatheusCavalari/kanvas/backend/internal/auth"
	"github.com/MatheusCavalari/kanvas/backend/internal/board"
	"github.com/MatheusCavalari/kanvas/backend/internal/card"
	"github.com/MatheusCavalari/kanvas/backend/internal/comment"
	"github.com/MatheusCavalari/kanvas/backend/internal/label"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/cache"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/config"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/gen"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/httpserver"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/jwt"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/metrics"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/middleware"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/worker"
	"github.com/MatheusCavalari/kanvas/backend/internal/realtime"
	"github.com/MatheusCavalari/kanvas/backend/internal/search"
	"github.com/MatheusCavalari/kanvas/backend/internal/webhook"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	metrics.Register()

	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	if err := runMigrations(cfg.DatabaseURL, cfg.MigrationsPath); err != nil {
		log.Fatalf("running migrations: %v", err)
	}

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connecting to database: %v", err)
	}

	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Fatalf("parsing redis url: %v", err)
	}
	redisClient := redis.NewClient(redisOpts)
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("connecting to redis: %v", err)
	}

	boardCache := cache.NewRedisCache(redisClient)

	queries := gen.New(pool)
	issuer := jwt.NewIssuer(cfg.JWTSecret, cfg.AccessTokenTTL)
	authMiddleware := middleware.Auth(issuer)

	authRepo := auth.NewPostgresRepository(queries)
	authService := auth.NewService(authRepo, issuer, cfg.RefreshTokenTTL)
	authHandler := auth.NewHandler(authService, cfg.SecureCookies)

	boardRepo := board.NewPostgresRepository(queries)
	userLookup := board.NewUserLookupAdapter(queries)
	boardService := board.NewService(boardRepo, userLookup)
	boardHandler := board.NewHandler(boardService)

	hub := realtime.NewHub()

	jobQueue := worker.NewRedisQueue(redisClient)

	cacheInvalidator := cache.NewInvalidator(boardCache, hub)
	webhookRepo := webhook.NewPostgresRepository(queries)
	webhookDispatcher := webhook.NewDispatcher(webhookRepo, jobQueue, cacheInvalidator)
	webhookService := webhook.NewService(webhookRepo, boardService)
	webhookHandler := webhook.NewHandler(webhookService)
	activityRepo := activity.NewPostgresRepository(queries)
	activityRecorder := activity.NewRecorder(activityRepo, webhookDispatcher)
	activityHandler := activity.NewHandler(activityRepo, boardService)

	cardRepo := card.NewPostgresRepository(queries)
	cardService := card.NewService(cardRepo, boardService, activityRecorder)
	cardHandler := card.NewHandler(cardService)

	labelRepo := label.NewPostgresRepository(queries)
	labelService := label.NewService(labelRepo, boardService, labelRepo, activityRecorder)
	labelHandler := label.NewHandler(labelService)

	commentRepo := comment.NewPostgresRepository(queries)
	commentService := comment.NewService(commentRepo, commentRepo, boardService, boardService, activityRecorder)
	commentHandler := comment.NewHandler(commentService)

	userNames := realtime.NewUserNameLookupAdapter(queries)
	realtimeHandler := realtime.NewHandler(hub, issuer, boardService, userNames, cfg.CORSAllowedOrigin)

	reaperCtx, stopReaper := context.WithCancel(context.Background())
	go hub.StartReaper(reaperCtx)

	searchHandler := search.NewHandler(queries, boardService)

	// Rate limiters: publicRateLimiter (by IP) protects every route,
	// including unauthenticated ones like /auth/login. The user-based
	// limiters run only after authMiddleware has populated the request
	// context with the caller's user ID, so they're composed into
	// protectedMiddleware below rather than applied as a blanket
	// router.Use — chi forbids registering top-level middleware once any
	// route has been added, which NewRouter already does (it registers
	// /metrics).
	publicRateLimiter := middleware.NewRateLimiter(redisClient, 20, time.Minute, middleware.IPKey)
	userWriteRateLimiter := middleware.NewRateLimiter(redisClient, 100, time.Minute, middleware.UserWriteKey)
	userReadRateLimiter := middleware.NewRateLimiter(redisClient, 300, time.Minute, middleware.UserReadKey)

	protectedMiddleware := func(next http.Handler) http.Handler {
		return authMiddleware(userWriteRateLimiter(userReadRateLimiter(next)))
	}

	router := httpserver.NewRouter(cfg.CORSAllowedOrigin)
	router.Group(func(r chi.Router) {
		r.Use(publicRateLimiter)
		authHandler.RegisterRoutes(r, protectedMiddleware)
		boardHandler.RegisterRoutes(r, protectedMiddleware)
		cardHandler.RegisterRoutes(r, protectedMiddleware)
		labelHandler.RegisterRoutes(r, protectedMiddleware)
		commentHandler.RegisterRoutes(r, protectedMiddleware)
		activityHandler.RegisterRoutes(r, protectedMiddleware)
		webhookHandler.RegisterRoutes(r, protectedMiddleware)
		searchHandler.RegisterRoutes(r, protectedMiddleware)
		realtimeHandler.RegisterPresenceRoute(r, protectedMiddleware)
	})
	realtimeHandler.RegisterWSRoute(router)

	healthChecker := httpserver.NewHealthChecker(pool, redisClient)
	router.Get("/livez", healthChecker.Livez)
	router.Get("/healthz", healthChecker.Livez)
	router.Get("/readyz", healthChecker.Readyz)

	jobWorker := worker.NewWorker(jobQueue)

	// Token cleanup job
	jobWorker.Register("token.cleanup", func(ctx context.Context, _ json.RawMessage) error {
		// Delete expired refresh tokens
		_, err := pool.Exec(ctx, "DELETE FROM refresh_tokens WHERE expires_at < now()")
		return err
	})

	// Webhook delivery job
	webhookDeliverer := webhook.NewDeliverer(webhookRepo)
	jobWorker.Register("webhook.deliver", webhookDeliverer)

	// Start worker
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()
	var workerWG sync.WaitGroup
	workerWG.Add(1)
	go func() {
		defer workerWG.Done()
		jobWorker.Run(workerCtx)
	}()

	// Token cleanup ticker
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-workerCtx.Done():
				return
			case <-ticker.C:
				_ = jobQueue.Enqueue(workerCtx, worker.Job{Type: "token.cleanup"})
			}
		}
	}()

	srv := &http.Server{Addr: ":" + cfg.Port, Handler: router}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("shutdown signal received")

		healthChecker.SetShuttingDown()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("http shutdown error: %v", err)
		}
		workerCancel()
		workerWG.Wait()
		stopReaper()
		hub.Close()
		redisClient.Close()
		pool.Close()
	}()

	log.Printf("listening on :%s", cfg.Port)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server error: %v", err)
	}
}

// runMigrations applies all pending SQL migrations from migrationsPath
// against databaseURL. It is idempotent — running it against an
// already-up-to-date database is a no-op.
func runMigrations(databaseURL, migrationsPath string) error {
	m, err := migrate.New("file://"+migrationsPath, databaseURL)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = m.Close()
	}()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}
