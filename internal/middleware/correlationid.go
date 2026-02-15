package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

type contextKey string

const CorrelationIDKey contextKey = "correlation_id"

// CorrelationID middleware injects or propagates correlation IDs
func CorrelationID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if correlation ID exists in header
		correlationID := r.Header.Get("X-Correlation-ID")
		if correlationID == "" {
			correlationID = uuid.New().String()
		}

		// Add to response header
		w.Header().Set("X-Correlation-ID", correlationID)

		// Add to context
		ctx := context.WithValue(r.Context(), CorrelationIDKey, correlationID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
