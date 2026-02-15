package middleware

import (
	"net/http"
	"sync/atomic"

	"weather-service/internal/metrics"

	"github.com/rs/zerolog/log"
)

var inflightCount int64

// LoadShedding middleware protects against saturation
// v3.1 Enhancement: Includes correlation ID logging and metric ordering
func LoadShedding(threshold int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			current := atomic.AddInt64(&inflightCount, 1)
			defer atomic.AddInt64(&inflightCount, -1)

			if current > int64(threshold) {
				// v3.1 Enhancement: Increment metric BEFORE returning
				metrics.LoadShedTotal.Inc()

				// v3.1 Enhancement: Log with correlation ID
				correlationID, _ := r.Context().Value(CorrelationIDKey).(string)
				log.Warn().
					Str("correlation_id", correlationID).
					Int64("inflight", current).
					Int("threshold", threshold).
					Msg("load shedding activated")

				http.Error(w, "service overloaded", http.StatusServiceUnavailable)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
