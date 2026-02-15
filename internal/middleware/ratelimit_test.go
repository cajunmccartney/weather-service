package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRateLimitExceeded(t *testing.T) {
	handler := RateLimit(1)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request - should succeed
	req1 := httptest.NewRequest("GET", "/test", nil)
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Errorf("first request: expected 200, got %d", w1.Code)
	}

	// Second immediate request - should succeed (burst = 2x rate)
	req2 := httptest.NewRequest("GET", "/test", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("second request: expected 200, got %d", w2.Code)
	}

	// Third immediate request - should be rate limited (exceeds burst)
	req3 := httptest.NewRequest("GET", "/test", nil)
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, req3)

	if w3.Code != http.StatusTooManyRequests {
		t.Errorf("third request: expected 429, got %d", w3.Code)
	}
}
