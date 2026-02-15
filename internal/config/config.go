package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	// API Configuration
	WeatherAPIKey        string
	WeatherAPIBaseURL    string
	WeatherAPIVersion    string // "2.5" or "3.0"
	UpstreamTimeoutSeconds int
	RequestTimeoutSeconds  int

	// Cache Configuration
	CacheTTLSeconds int

	// Rate Limiting
	RateLimitRPS int

	// Load Shedding
	LoadShedThreshold int

	// Server Configuration
	Port     string
	LogLevel string
}

// Validate checks if configuration values are valid
func (cfg *Config) Validate() error {
	var errs []string

	// Validate API key
	if cfg.WeatherAPIKey == "" {
		errs = append(errs, "WEATHER_API_KEY is required")
	}

	// Validate timeouts
	if cfg.UpstreamTimeoutSeconds < 1 {
		errs = append(errs, "UPSTREAM_TIMEOUT_SECONDS must be at least 1")
	}
	if cfg.RequestTimeoutSeconds < 1 {
		errs = append(errs, "REQUEST_TIMEOUT_SECONDS must be at least 1")
	}
	if cfg.UpstreamTimeoutSeconds >= cfg.RequestTimeoutSeconds {
		errs = append(errs, "UPSTREAM_TIMEOUT_SECONDS must be less than REQUEST_TIMEOUT_SECONDS")
	}

	// Validate cache
	if cfg.CacheTTLSeconds < 1 {
		errs = append(errs, "CACHE_TTL_SECONDS must be at least 1")
	}

	// Validate rate limiting
	if cfg.RateLimitRPS < 1 {
		errs = append(errs, "RATE_LIMIT_RPS must be at least 1")
	}

	// Validate load shedding
	if cfg.LoadShedThreshold < 1 {
		errs = append(errs, "LOAD_SHED_THRESHOLD must be at least 1")
	}

	// Validate port
	port, err := strconv.Atoi(cfg.Port)
	if err != nil || port < 1 || port > 65535 {
		errs = append(errs, "PORT must be a valid port number (1-65535)")
	}

	// Validate log level
	validLevels := []string{"trace", "debug", "info", "warn", "error", "fatal", "panic"}
	validLevel := false
	for _, level := range validLevels {
		if cfg.LogLevel == level {
			validLevel = true
			break
		}
	}
	if !validLevel {
		errs = append(errs, fmt.Sprintf("LOG_LEVEL must be one of: %s", strings.Join(validLevels, ", ")))
	}

	// Validate API version
	if cfg.WeatherAPIVersion != "2.5" && cfg.WeatherAPIVersion != "3.0" {
		errs = append(errs, "WEATHER_API_VERSION must be '2.5' or '3.0'")
	}

	if len(errs) > 0 {
		return fmt.Errorf("configuration validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}

	return nil
}

func Load() (*Config, error) {
	cfg := &Config{
		WeatherAPIKey:         getEnv("WEATHER_API_KEY", ""),
		WeatherAPIBaseURL:     getEnv("WEATHER_API_BASE_URL", ""),
		WeatherAPIVersion:     getEnv("WEATHER_API_VERSION", "2.5"),
		UpstreamTimeoutSeconds: getEnvInt("UPSTREAM_TIMEOUT_SECONDS", 5),
		RequestTimeoutSeconds:  getEnvInt("REQUEST_TIMEOUT_SECONDS", 10),
		CacheTTLSeconds:       getEnvInt("CACHE_TTL_SECONDS", 300),
		RateLimitRPS:          getEnvInt("RATE_LIMIT_RPS", 50),
		LoadShedThreshold:     getEnvInt("LOAD_SHED_THRESHOLD", 100),
		Port:                  getEnv("PORT", "8080"),
		LogLevel:              getEnv("LOG_LEVEL", "info"),
	}

	// Set default base URL based on API version if not explicitly provided
	if cfg.WeatherAPIBaseURL == "" {
		if cfg.WeatherAPIVersion == "3.0" {
			cfg.WeatherAPIBaseURL = "https://api.openweathermap.org/data/3.0"
		} else {
			cfg.WeatherAPIBaseURL = "https://api.openweathermap.org/data/2.5"
		}
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
