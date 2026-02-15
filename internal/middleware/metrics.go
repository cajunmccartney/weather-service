package middleware

import (
	"net/http"
	"strconv"
	"time"

	"weather-service/internal/metrics"
)

// Metrics middleware for Prometheus instrumentation
func Metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Track inflight requests
		metrics.HTTPInflightRequests.WithLabelValues(r.URL.Path).Inc()
		defer metrics.HTTPInflightRequests.WithLabelValues(r.URL.Path).Dec()

		// Wrap response writer
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		// Record metrics
		duration := time.Since(start)
		status := strconv.Itoa(wrapped.statusCode)

		metrics.HTTPRequestsTotal.WithLabelValues(r.Method, r.URL.Path, status).Inc()
		metrics.HTTPRequestDuration.WithLabelValues(r.URL.Path).Observe(duration.Seconds())
	})
}
