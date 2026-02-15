package weather

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"weather-service/internal/config"
)

func TestRetryLogic_Retries5xx(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle geocoding endpoint
		if strings.Contains(r.URL.Path, "/geo/1.0/direct") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[{"name":"TestCity","lat":51.5074,"lon":-0.1278,"country":"GB"}]`))
			return
		}

		// Handle One Call endpoint with retries
		if strings.Contains(r.URL.Path, "/onecall") {
			attempts++
			if attempts < 3 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"current":{"temp":72,"humidity":50,"wind_speed":5,"weather":[{"main":"Clear"}]}}`))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cfg := &config.Config{
		WeatherAPIBaseURL:      server.URL,
		WeatherAPIKey:          "test",
		UpstreamTimeoutSeconds: 5,
	}

	client := NewClient(cfg)
	ctx := context.Background()

	data, err := client.FetchWeather(ctx, "test")
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}

	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}

	if data == nil {
		t.Fatal("expected data, got nil")
	}
}

func TestRetryLogic_DoesNotRetry4xx(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle geocoding endpoint
		if strings.Contains(r.URL.Path, "/geo/1.0/direct") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[{"name":"TestCity","lat":51.5074,"lon":-0.1278,"country":"GB"}]`))
			return
		}

		// Handle One Call endpoint - return 404
		if strings.Contains(r.URL.Path, "/onecall") {
			attempts++
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cfg := &config.Config{
		WeatherAPIBaseURL:      server.URL,
		WeatherAPIKey:          "test",
		UpstreamTimeoutSeconds: 5,
	}

	client := NewClient(cfg)
	ctx := context.Background()

	_, err := client.FetchWeather(ctx, "test")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if attempts != 1 {
		t.Errorf("expected 1 attempt (no retry on 4xx), got %d", attempts)
	}
}

func TestContextTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle geocoding endpoint
		if strings.Contains(r.URL.Path, "/geo/1.0/direct") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[{"name":"TestCity","lat":51.5074,"lon":-0.1278,"country":"GB"}]`))
			return
		}

		// Handle One Call endpoint - sleep to trigger timeout
		if strings.Contains(r.URL.Path, "/onecall") {
			time.Sleep(2 * time.Second)
			w.WriteHeader(http.StatusOK)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cfg := &config.Config{
		WeatherAPIBaseURL:      server.URL,
		WeatherAPIKey:          "test",
		UpstreamTimeoutSeconds: 1,
	}

	client := NewClient(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, err := client.FetchWeather(ctx, "test")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestFetchWeather_WithCoordinates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Should NOT call geocoding endpoint
		if strings.Contains(r.URL.Path, "/geo/1.0/direct") {
			t.Error("geocoding should not be called when coordinates provided")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Should call onecall endpoint
		if strings.Contains(r.URL.Path, "/onecall") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"current":{"temp":72,"humidity":50,"wind_speed":5,"weather":[{"main":"Clear"}]}}`))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cfg := &config.Config{
		WeatherAPIBaseURL:      server.URL,
		WeatherAPIKey:          "test",
		UpstreamTimeoutSeconds: 5,
	}

	client := NewClient(cfg)
	ctx := context.Background()

	// Test with coordinates (should skip geocoding)
	data, err := client.FetchWeather(ctx, "51.5074,-0.1278")
	if err != nil {
		t.Fatalf("expected success with coordinates, got: %v", err)
	}

	if data == nil {
		t.Fatal("expected data, got nil")
	}
}

func TestParseCoordinates(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantLat float64
		wantLon float64
		wantOk  bool
	}{
		{"valid coords", "51.5074,-0.1278", 51.5074, -0.1278, true},
		{"valid coords with spaces", "51.5074, -0.1278", 51.5074, -0.1278, true},
		{"nyc coords", "40.7128,-74.0060", 40.7128, -74.0060, true},
		{"invalid - city name", "London", 0, 0, false},
		{"invalid - too many parts", "51.5074,-0.1278,extra", 0, 0, false},
		{"invalid - not numbers", "abc,def", 0, 0, false},
		{"invalid - lat out of range", "91.0,-0.1278", 0, 0, false},
		{"invalid - lon out of range", "51.5074,181.0", 0, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lat, lon, ok := parseCoordinates(tt.input)
			if ok != tt.wantOk {
				t.Errorf("parseCoordinates() ok = %v, want %v", ok, tt.wantOk)
			}
			if ok && (lat != tt.wantLat || lon != tt.wantLon) {
				t.Errorf("parseCoordinates() = (%v, %v), want (%v, %v)", lat, lon, tt.wantLat, tt.wantLon)
			}
		})
	}
}
