package config

import (
	"os"
	"testing"
)

func TestValidate_Success(t *testing.T) {
	cfg := &Config{
		WeatherAPIKey:         "test-key-123",
		WeatherAPIVersion:     "2.5",
		UpstreamTimeoutSeconds: 5,
		RequestTimeoutSeconds:  10,
		CacheTTLSeconds:       300,
		RateLimitRPS:          50,
		LoadShedThreshold:     100,
		Port:                  "8080",
		LogLevel:              "info",
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() failed with valid config: %v", err)
	}
}

func TestValidate_InvalidTimeoutRelationship(t *testing.T) {
	cfg := &Config{
		WeatherAPIKey:         "test-key",
		WeatherAPIVersion:     "2.5",
		UpstreamTimeoutSeconds: 10, // Invalid: >= request timeout
		RequestTimeoutSeconds:  5,
		CacheTTLSeconds:       300,
		RateLimitRPS:          50,
		LoadShedThreshold:     100,
		Port:                  "8080",
		LogLevel:              "info",
	}

	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail when upstream timeout >= request timeout")
	}
}

func TestValidate_NegativeTTL(t *testing.T) {
	cfg := &Config{
		WeatherAPIKey:         "test-key",
		WeatherAPIVersion:     "2.5",
		UpstreamTimeoutSeconds: 5,
		RequestTimeoutSeconds:  10,
		CacheTTLSeconds:       -1, // Invalid
		RateLimitRPS:          50,
		LoadShedThreshold:     100,
		Port:                  "8080",
		LogLevel:              "info",
	}

	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail with negative TTL")
	}
}

func TestValidate_InvalidPort(t *testing.T) {
	cfg := &Config{
		WeatherAPIKey:         "test-key",
		WeatherAPIVersion:     "2.5",
		UpstreamTimeoutSeconds: 5,
		RequestTimeoutSeconds:  10,
		CacheTTLSeconds:       300,
		RateLimitRPS:          50,
		LoadShedThreshold:     100,
		Port:                  "invalid", // Invalid
		LogLevel:              "info",
	}

	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail with invalid port")
	}
}

func TestValidate_InvalidAPIVersion(t *testing.T) {
	cfg := &Config{
		WeatherAPIKey:         "test-key",
		WeatherAPIVersion:     "2.0", // Invalid
		UpstreamTimeoutSeconds: 5,
		RequestTimeoutSeconds:  10,
		CacheTTLSeconds:       300,
		RateLimitRPS:          50,
		LoadShedThreshold:     100,
		Port:                  "8080",
		LogLevel:              "info",
	}

	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail with invalid API version")
	}
}

func TestLoad_DefaultVersion(t *testing.T) {
	// Don't set WEATHER_API_VERSION env var
	os.Unsetenv("WEATHER_API_VERSION")
	os.Setenv("WEATHER_API_KEY", "test-key")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.WeatherAPIVersion != "2.5" {
		t.Errorf("Default API version should be 2.5, got: %s", cfg.WeatherAPIVersion)
	}

	if cfg.WeatherAPIBaseURL != "https://api.openweathermap.org/data/2.5" {
		t.Errorf("Default base URL should be 2.5 endpoint, got: %s", cfg.WeatherAPIBaseURL)
	}
}
