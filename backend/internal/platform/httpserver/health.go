package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// HealthChecker backs the /livez and /readyz endpoints. Livez reports
// whether the process is alive and not shutting down; Readyz reports
// whether its dependencies (database, cache) are reachable.
type HealthChecker struct {
	pool         *pgxpool.Pool
	redis        *redis.Client
	shuttingDown atomic.Bool
}

func NewHealthChecker(pool *pgxpool.Pool, redis *redis.Client) *HealthChecker {
	return &HealthChecker{pool: pool, redis: redis}
}

// SetShuttingDown marks the process as shutting down so Livez starts
// reporting unavailable. Called once, at the start of graceful shutdown.
func (hc *HealthChecker) SetShuttingDown() {
	hc.shuttingDown.Store(true)
}

func (hc *HealthChecker) Livez(w http.ResponseWriter, r *http.Request) {
	if hc.shuttingDown.Load() {
		writeHealthJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "shutting_down"})
		return
	}
	writeHealthJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (hc *HealthChecker) Readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	checks := map[string]string{}
	ready := true

	if hc.pool == nil {
		checks["db"] = "not_configured"
	} else if err := hc.pool.Ping(ctx); err != nil {
		checks["db"] = "error: " + err.Error()
		ready = false
	} else {
		checks["db"] = "ok"
	}

	if hc.redis == nil {
		checks["redis"] = "not_configured"
	} else if err := hc.redis.Ping(ctx).Err(); err != nil {
		checks["redis"] = "error: " + err.Error()
		ready = false
	} else {
		checks["redis"] = "ok"
	}

	status := http.StatusOK
	statusText := "ready"
	if !ready {
		status = http.StatusServiceUnavailable
		statusText = "not_ready"
	}
	result := map[string]interface{}{"status": statusText, "checks": checks}
	writeHealthJSON(w, status, result)
}

func writeHealthJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
