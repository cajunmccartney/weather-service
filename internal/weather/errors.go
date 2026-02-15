package weather

import "fmt"

// APIError represents a structured error from the weather API
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("weather API error (status %d): %s", e.StatusCode, e.Message)
}

// IsRetryable returns true if this error should trigger a retry
func (e *APIError) IsRetryable() bool {
	// Retry on 5xx server errors and 429 rate limits
	// DO NOT retry other 4xx client errors (prevents quota exhaustion)
	return e.StatusCode >= 500 || e.StatusCode == 429
}

// NetworkError represents a network-level failure (always retryable)
type NetworkError struct {
	Op  string
	Err error
}

func (e *NetworkError) Error() string {
	return fmt.Sprintf("network error during %s: %v", e.Op, e.Err)
}

func (e *NetworkError) Unwrap() error {
	return e.Err
}

func (e *NetworkError) IsRetryable() bool {
	return true // Network errors are always retryable
}
