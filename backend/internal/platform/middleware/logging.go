package middleware

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// correlationIDKey reuses the contextKey type defined in auth.go so all
// context values in this package share one type (avoids accidental
// collisions between differently-typed string keys).
const correlationIDKey contextKey = "correlationID"

// CorrelationIDFromContext returns the correlation ID stored in ctx, or
// the empty string if none is present.
func CorrelationIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(correlationIDKey).(string)
	return id
}

// CorrelationID is HTTP middleware that ensures every request carries a
// correlation ID: it reuses the inbound X-Correlation-ID header when
// present, otherwise generates a new UUID. The ID is echoed back on the
// response and stored in the request context for downstream handlers and
// logging middleware to read.
func CorrelationID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cid := r.Header.Get("X-Correlation-ID")
		if cid == "" {
			cid = uuid.New().String()
		}
		w.Header().Set("X-Correlation-ID", cid)
		ctx := context.WithValue(r.Context(), correlationIDKey, cid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// statusRecorder wraps an http.ResponseWriter to capture the status code
// written by the handler, so middleware running after the handler (e.g.
// logging, metrics) can observe it.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.status = code
	sr.ResponseWriter.WriteHeader(code)
}

// Hijack delegates to the underlying ResponseWriter's Hijacker
// implementation, if any. This is required for WebSocket upgrades (and
// any other protocol that takes over the raw connection) to keep working
// when statusRecorder sits in the middleware chain in front of them.
func (sr *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := sr.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("underlying ResponseWriter does not implement http.Hijacker")
	}
	return hijacker.Hijack()
}

// SlogLogger is HTTP middleware that logs one structured (JSON via the
// default slog handler) line per completed request: method, path, status,
// duration, and correlation ID.
func SlogLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		duration := time.Since(start)

		slog.Info("request completed",
			"correlation_id", CorrelationIDFromContext(r.Context()),
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", duration.Milliseconds(),
		)
	})
}
