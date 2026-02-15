package weather

import (
	"errors"
	"testing"
)

func TestAPIError_IsRetryable(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		want       bool
	}{
		{"400 Bad Request", 400, false},
		{"401 Unauthorized", 401, false},
		{"403 Forbidden", 403, false},
		{"404 Not Found", 404, false},
		{"429 Too Many Requests", 429, true},
		{"500 Internal Server Error", 500, true},
		{"502 Bad Gateway", 502, true},
		{"503 Service Unavailable", 503, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &APIError{StatusCode: tt.statusCode, Message: "test"}
			if got := err.IsRetryable(); got != tt.want {
				t.Errorf("IsRetryable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNetworkError_IsRetryable(t *testing.T) {
	err := &NetworkError{Op: "dial", Err: errors.New("connection refused")}
	if !err.IsRetryable() {
		t.Error("NetworkError should always be retryable")
	}
}
