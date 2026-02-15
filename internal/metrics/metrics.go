package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// HTTP Layer Metrics
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests by method, route, and status",
		},
		[]string{"method", "route", "status"},
	)

	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency distribution",
			Buckets: prometheus.DefBuckets, // .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10
		},
		[]string{"route"},
	)

	HTTPInflightRequests = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "http_inflight_requests",
			Help: "Current number of inflight HTTP requests",
		},
		[]string{"route"},
	)

	// External API Metrics
	ExternalAPIRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "external_api_requests_total",
			Help: "Total requests to external APIs",
		},
		[]string{"endpoint", "status"},
	)

	ExternalAPIDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "external_api_duration_seconds",
			Help:    "External API request latency",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"endpoint"},
	)

	ExternalAPIFailuresTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "external_api_failures_total",
			Help: "Total external API failures by error type",
		},
		[]string{"endpoint", "error_type"},
	)

	ExternalAPIRetriesTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "external_api_retries_total",
			Help: "Total number of retry attempts",
		},
	)

	// Cache Metrics
	CacheHitsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_hits_total",
			Help: "Total cache hits",
		},
		[]string{"location"},
	)

	CacheMissesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_misses_total",
			Help: "Total cache misses",
		},
		[]string{"location"},
	)

	CacheSizeBytes = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "cache_size_bytes",
			Help: "Current cache size in bytes",
		},
	)

	CacheEvictionsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "cache_evictions_total",
			Help: "Total cache evictions",
		},
	)

	// v3.1 Enhancement: Stale-on-error metric
	CacheStaleServedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "cache_stale_served_total",
			Help: "Total stale cache entries served on upstream failure",
		},
	)

	// Rate Limiting Metrics
	RateLimitExceededTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rate_limit_exceeded_total",
			Help: "Total rate limit exceeded events",
		},
		[]string{"client_id"},
	)

	// v3.1 Enhancement: Load shedding metric
	LoadShedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "load_shed_total",
			Help: "Total requests shed due to saturation",
		},
	)

	// Service Health Metrics
	ServiceUp = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "service_up",
			Help: "Service availability (1 = up, 0 = down)",
		},
	)
)

// Init initializes metrics
// Note: Go runtime collectors (go_*, process_*) are automatically registered
// by the Prometheus client library's default registry, so we don't need to
// register them manually here.
func Init() {
	// Set service as up
	ServiceUp.Set(1)
}
