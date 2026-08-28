// Package metrics defines the Prometheus metrics collected across the
// application and exposes a single Register function to wire them into
// the default registry.
package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	// HTTPRequestsTotal counts every HTTP request handled by the server,
	// labeled by method, route pattern, and response status code.
	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total HTTP requests processed",
		},
		[]string{"method", "path", "status"},
	)

	// HTTPRequestDuration observes HTTP request latency in seconds,
	// labeled by method and route pattern.
	HTTPRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	// WebSocketConnectionsActive tracks the current number of open
	// WebSocket connections.
	WebSocketConnectionsActive = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "websocket_connections_active",
			Help: "Current active WebSocket connections",
		},
	)

	// CacheOperationsTotal counts cache operations, labeled by
	// operation (e.g. "get", "set", "invalidate") and result
	// (e.g. "hit", "miss", "error").
	CacheOperationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_operations_total",
			Help: "Total cache operations",
		},
		[]string{"operation", "result"},
	)

	// WorkerJobsTotal counts background jobs processed, labeled by
	// job type and result.
	WorkerJobsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "worker_jobs_processed_total",
			Help: "Total background jobs processed",
		},
		[]string{"job_type", "result"},
	)

	// WebhookDeliveriesTotal counts outbound webhook delivery attempts,
	// labeled by outcome status.
	WebhookDeliveriesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "webhook_deliveries_total",
			Help: "Total webhook deliveries",
		},
		[]string{"status"},
	)
)

// Register registers all application metrics with the default Prometheus
// registry. It must be called exactly once, before the HTTP server starts
// serving /metrics.
func Register() {
	prometheus.MustRegister(
		HTTPRequestsTotal,
		HTTPRequestDuration,
		WebSocketConnectionsActive,
		CacheOperationsTotal,
		WorkerJobsTotal,
		WebhookDeliveriesTotal,
	)
}
