package middleware

import (
	"net/http"

	"weather-service/internal/metrics"

	"golang.org/x/time/rate"
)

// RateLimit middleware using token bucket algorithm
func RateLimit(rps int) func(http.Handler) http.Handler {
	limiter := rate.NewLimiter(rate.Limit(rps), rps*2) // burst = 2x rate

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.Allow() {
				// v3.1 Enhancement: Track client_id (use IP as simple identifier)
				clientID := r.RemoteAddr
				metrics.RateLimitExceededTotal.WithLabelValues(clientID).Inc()

				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
