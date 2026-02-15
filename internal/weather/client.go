package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"weather-service/internal/config"
	"weather-service/internal/metrics"
	"weather-service/internal/middleware"

	"github.com/rs/zerolog/log"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

type Client struct {
	cfg        *config.Config
	httpClient *http.Client
}

func NewClient(cfg *config.Config) *Client {
	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: time.Duration(cfg.UpstreamTimeoutSeconds) * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				MaxConnsPerHost:     10,
				IdleConnTimeout:     90 * time.Second,
				TLSHandshakeTimeout: 10 * time.Second,
				DisableKeepAlives:   false,
			},
		},
	}
}

// parseCoordinates attempts to parse input as "lat,lon" coordinates
// Returns lat, lon, and true if successful; otherwise returns 0, 0, false
func parseCoordinates(input string) (lat, lon float64, ok bool) {
	parts := strings.Split(input, ",")
	if len(parts) != 2 {
		return 0, 0, false
	}

	lat, latErr := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	lon, lonErr := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)

	if latErr != nil || lonErr != nil {
		return 0, 0, false
	}

	// Validate coordinate ranges
	if lat < -90 || lat > 90 {
		return 0, 0, false // Invalid latitude
	}
	if lon < -180 || lon > 180 {
		return 0, 0, false // Invalid longitude
	}

	return lat, lon, true
}

// fetchWeather2_5 fetches weather using the legacy 2.5 API (city name only)
func (c *Client) fetchWeather2_5(ctx context.Context, location string) (*WeatherData, error) {
	maxRetries := 3
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 100ms, 200ms, 400ms
			baseBackoff := time.Duration(math.Pow(2, float64(attempt-1))*100) * time.Millisecond

			// Jitter: ±25% of backoff to prevent thundering herd
			jitterRange := baseBackoff / 4
			jitter := time.Duration(rand.Int63n(int64(jitterRange*2))) - jitterRange

			sleepDuration := baseBackoff + jitter
			time.Sleep(sleepDuration)

			metrics.ExternalAPIRetriesTotal.Inc()
			logger := log.With().Int("attempt", attempt).Dur("backoff", baseBackoff).Dur("jitter", jitter).Logger()
			if correlationID := ctx.Value(middleware.CorrelationIDKey); correlationID != nil {
				logger = logger.With().Str("correlation_id", correlationID.(string)).Logger()
			}
			logger.Debug().Msg("retrying request")
		}

		data, err := c.doWeather2_5Request(ctx, location)
		if err == nil {
			return data, nil
		}

		lastErr = err

		// DO NOT retry 4xx errors
		if apiErr, ok := lastErr.(*APIError); ok && !apiErr.IsRetryable() {
			log.Debug().Int("status_code", apiErr.StatusCode).Msg("non-retryable error, aborting")
			return nil, lastErr
		}
	}

	logger := log.With().Err(lastErr).Str("location", location).Logger()
	if correlationID := ctx.Value(middleware.CorrelationIDKey); correlationID != nil {
		logger = logger.With().Str("correlation_id", correlationID.(string)).Logger()
	}
	logger.Error().Int("max_retries", maxRetries).Msg("all retries exhausted")
	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// doWeather2_5Request makes a single request to the 2.5 API
func (c *Client) doWeather2_5Request(ctx context.Context, location string) (*WeatherData, error) {
	start := time.Now()

	// Build URL with proper query parameter encoding
	baseURL := c.cfg.WeatherAPIBaseURL + "/weather"
	params := url.Values{}
	params.Add("q", location)
	params.Add("appid", c.cfg.WeatherAPIKey)
	params.Add("units", "imperial")
	apiURL := baseURL + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		metrics.ExternalAPIFailuresTotal.WithLabelValues("openweathermap_2.5", "request_creation").Inc()
		return nil, &NetworkError{Op: "request_creation", Err: err}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		metrics.ExternalAPIFailuresTotal.WithLabelValues("openweathermap_2.5", "network").Inc()
		metrics.ExternalAPIRequestsTotal.WithLabelValues("openweathermap_2.5", "error").Inc()
		return nil, &NetworkError{Op: "http_request", Err: err}
	}
	defer resp.Body.Close()

	duration := time.Since(start)
	metrics.ExternalAPIDuration.WithLabelValues("openweathermap_2.5").Observe(duration.Seconds())

	statusClass := fmt.Sprintf("%dxx", resp.StatusCode/100)
	metrics.ExternalAPIRequestsTotal.WithLabelValues("openweathermap_2.5", statusClass).Inc()

	if resp.StatusCode != http.StatusOK {
		metrics.ExternalAPIFailuresTotal.WithLabelValues("openweathermap_2.5", statusClass).Inc()
		body, _ := io.ReadAll(resp.Body)
		logger := log.With().
			Int("status_code", resp.StatusCode).
			Str("response_body", string(body)).
			Logger()
		if correlationID := ctx.Value(middleware.CorrelationIDKey); correlationID != nil {
			logger = logger.With().Str("correlation_id", correlationID.(string)).Logger()
		}
		logger.Error().Msg("Weather API 2.5 returned non-200 status")

		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("Weather API 2.5 returned status: %s", statusClass),
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		metrics.ExternalAPIFailuresTotal.WithLabelValues("openweathermap_2.5", "read_error").Inc()
		return nil, err
	}

	// 2.5 API returns WeatherData directly (no nested current object)
	var weatherData WeatherData
	if err := json.Unmarshal(body, &weatherData); err != nil {
		metrics.ExternalAPIFailuresTotal.WithLabelValues("openweathermap_2.5", "parse_error").Inc()
		return nil, err
	}

	return &weatherData, nil
}

type WeatherData struct {
	Main struct {
		Temp     float64 `json:"temp"`
		Humidity int     `json:"humidity"`
	} `json:"main"`
	Weather []struct {
		Main string `json:"main"`
	} `json:"weather"`
	Wind struct {
		Speed float64 `json:"speed"`
	} `json:"wind"`
}

// GeocodingResult represents a location from the Geocoding API
type GeocodingResult struct {
	Name    string  `json:"name"`
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
	Country string  `json:"country"`
	State   string  `json:"state,omitempty"`
}

// OneCallResponse represents the One Call API 3.0 response
type OneCallResponse struct {
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
	Current struct {
		Temp      float64 `json:"temp"`
		FeelsLike float64 `json:"feels_like"`
		Humidity  int     `json:"humidity"`
		Pressure  int     `json:"pressure"`
		WindSpeed float64 `json:"wind_speed"`
		Weather   []struct {
			Main        string `json:"main"`
			Description string `json:"description"`
		} `json:"weather"`
	} `json:"current"`
}

// geocodeLocation converts a location name to lat/lon coordinates
func (c *Client) geocodeLocation(ctx context.Context, location string) (*GeocodingResult, error) {
	// Properly construct URL with query parameters to handle spaces and special characters
	baseURL := "http://api.openweathermap.org/geo/1.0/direct"
	params := url.Values{}
	params.Add("q", location)
	params.Add("limit", "1")
	params.Add("appid", c.cfg.WeatherAPIKey)
	apiURL := baseURL + "?" + params.Encode()

	// Log geocoding attempt
	logger := log.With().Str("location", location).Logger()
	if correlationID := ctx.Value(middleware.CorrelationIDKey); correlationID != nil {
		logger = logger.With().Str("correlation_id", correlationID.(string)).Logger()
	}
	logger.Debug().Msg("geocoding location")

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, &NetworkError{Op: "geocoding_request_creation", Err: err}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &NetworkError{Op: "geocoding_request", Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger := log.With().
			Int("status_code", resp.StatusCode).
			Str("response_body", string(body)).
			Logger()
		if correlationID := ctx.Value(middleware.CorrelationIDKey); correlationID != nil {
			logger = logger.With().Str("correlation_id", correlationID.(string)).Logger()
		}
		logger.Error().Msg("geocoding API returned non-200 status")

		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Message:    "geocoding API returned non-200 status",
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &NetworkError{Op: "geocoding_read_body", Err: err}
	}

	var results []GeocodingResult
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("failed to parse geocoding response: %w", err)
	}

	if len(results) == 0 {
		return nil, &APIError{
			StatusCode: 404,
			Message:    fmt.Sprintf("location not found: %s", location),
		}
	}

	return &results[0], nil
}

// fetchWeatherWithRetry fetches weather data with retry logic
func (c *Client) fetchWeatherWithRetry(ctx context.Context, lat, lon float64) (*WeatherData, error) {
	maxRetries := 3
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 100ms, 200ms, 400ms
			baseBackoff := time.Duration(math.Pow(2, float64(attempt-1))*100) * time.Millisecond

			// Jitter: ±25% of backoff to prevent thundering herd
			jitterRange := baseBackoff / 4
			jitter := time.Duration(rand.Int63n(int64(jitterRange*2))) - jitterRange

			sleepDuration := baseBackoff + jitter
			time.Sleep(sleepDuration)

			metrics.ExternalAPIRetriesTotal.Inc()
			logger := log.With().Int("attempt", attempt).Dur("backoff", baseBackoff).Dur("jitter", jitter).Logger()
			if correlationID := ctx.Value(middleware.CorrelationIDKey); correlationID != nil {
				logger = logger.With().Str("correlation_id", correlationID.(string)).Logger()
			}
			logger.Debug().Msg("retrying request")
		}

		data, err := c.doOneCallRequest(ctx, lat, lon)
		if err == nil {
			return data, nil
		}

		lastErr = err

		// DO NOT retry 4xx errors (prevents quota exhaustion)
		if apiErr, ok := lastErr.(*APIError); ok && !apiErr.IsRetryable() {
			log.Debug().Int("status_code", apiErr.StatusCode).Msg("non-retryable error, aborting")
			return nil, lastErr
		}
	}

	// Log when all retries are exhausted
	logger := log.With().Err(lastErr).Float64("lat", lat).Float64("lon", lon).Logger()
	if correlationID := ctx.Value(middleware.CorrelationIDKey); correlationID != nil {
		logger = logger.With().Str("correlation_id", correlationID.(string)).Logger()
	}
	logger.Error().Int("max_retries", maxRetries).Msg("all retries exhausted")
	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// FetchWeather fetches weather data for a location
// Supports API versions 2.5 (default, free) and 3.0 (requires subscription)
// For 3.0: Supports both location names and coordinates (lat,lon)
// For 2.5: Supports location names only
func (c *Client) FetchWeather(ctx context.Context, location string) (*WeatherData, error) {
	// Route to appropriate API version
	if c.cfg.WeatherAPIVersion == "3.0" {
		return c.fetchWeather3_0(ctx, location)
	}

	// Default: Use 2.5 API (free, no credit card required)
	return c.fetchWeather2_5(ctx, location)
}

// fetchWeather3_0 fetches weather using One Call API 3.0
// Supports both location names (geocoded) and coordinates (direct)
func (c *Client) fetchWeather3_0(ctx context.Context, location string) (*WeatherData, error) {
	// Try to parse as coordinates first
	if lat, lon, ok := parseCoordinates(location); ok {
		// Input is coordinates - skip geocoding
		logger := log.With().
			Float64("lat", lat).
			Float64("lon", lon).
			Str("input_type", "coordinates").
			Logger()
		if correlationID := ctx.Value(middleware.CorrelationIDKey); correlationID != nil {
			logger = logger.With().Str("correlation_id", correlationID.(string)).Logger()
		}
		logger.Debug().Msg("using provided coordinates, skipping geocoding")

		return c.fetchWeatherWithRetry(ctx, lat, lon)
	}

	// Input is location name - geocode first
	logger := log.With().
		Str("location", location).
		Str("input_type", "location_name").
		Logger()
	if correlationID := ctx.Value(middleware.CorrelationIDKey); correlationID != nil {
		logger = logger.With().Str("correlation_id", correlationID.(string)).Logger()
	}
	logger.Debug().Msg("geocoding location")

	geo, err := c.geocodeLocation(ctx, location)
	if err != nil {
		// Error logging already handled in geocodeLocation
		return nil, fmt.Errorf("geocoding failed: %w", err)
	}

	logger.Debug().
		Float64("lat", geo.Lat).
		Float64("lon", geo.Lon).
		Msg("geocoded location")

	return c.fetchWeatherWithRetry(ctx, geo.Lat, geo.Lon)
}

// doOneCallRequest fetches weather data from One Call API 3.0
func (c *Client) doOneCallRequest(ctx context.Context, lat, lon float64) (*WeatherData, error) {
	start := time.Now()

	// Use One Call API 3.0 with only current weather (exclude forecasts)
	url := fmt.Sprintf("%s/onecall?lat=%f&lon=%f&appid=%s&units=imperial&exclude=minutely,hourly,daily,alerts",
		c.cfg.WeatherAPIBaseURL,
		lat,
		lon,
		c.cfg.WeatherAPIKey,
	)

	// Log the full URL (with masked API key) for debugging
	maskedURL := fmt.Sprintf("%s/onecall?lat=%f&lon=%f&appid=***MASKED***&units=imperial&exclude=minutely,hourly,daily,alerts",
		c.cfg.WeatherAPIBaseURL,
		lat,
		lon,
	)
	logger := log.With().Str("url", maskedURL).Logger()
	if correlationID := ctx.Value(middleware.CorrelationIDKey); correlationID != nil {
		logger = logger.With().Str("correlation_id", correlationID.(string)).Logger()
	}
	logger.Debug().Msg("making One Call API request")

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		metrics.ExternalAPIFailuresTotal.WithLabelValues("openweathermap_3.0", "request_creation").Inc()
		logger := log.With().Err(err).Logger()
		if correlationID := ctx.Value(middleware.CorrelationIDKey); correlationID != nil {
			logger = logger.With().Str("correlation_id", correlationID.(string)).Logger()
		}
		logger.Error().Msg("failed to create HTTP request")
		return nil, &NetworkError{Op: "request_creation", Err: err}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Network or timeout error
		metrics.ExternalAPIFailuresTotal.WithLabelValues("openweathermap_3.0", "network").Inc()
		metrics.ExternalAPIRequestsTotal.WithLabelValues("openweathermap_3.0", "error").Inc()
		return nil, &NetworkError{Op: "http_request", Err: err}
	}
	defer resp.Body.Close()

	duration := time.Since(start)
	metrics.ExternalAPIDuration.WithLabelValues("openweathermap_3.0").Observe(duration.Seconds())

	// Record status
	statusClass := fmt.Sprintf("%dxx", resp.StatusCode/100)
	metrics.ExternalAPIRequestsTotal.WithLabelValues("openweathermap_3.0", statusClass).Inc()

	if resp.StatusCode != http.StatusOK {
		metrics.ExternalAPIFailuresTotal.WithLabelValues("openweathermap_3.0", statusClass).Inc()

		// Log the response body for 401 errors to see the exact error message
		body, _ := io.ReadAll(resp.Body)
		logger := log.With().
			Int("status_code", resp.StatusCode).
			Str("response_body", string(body)).
			Logger()
		if correlationID := ctx.Value(middleware.CorrelationIDKey); correlationID != nil {
			logger = logger.With().Str("correlation_id", correlationID.(string)).Logger()
		}
		logger.Error().Msg("One Call API returned non-200 status")

		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("One Call API returned non-200 status: %s", statusClass),
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		metrics.ExternalAPIFailuresTotal.WithLabelValues("openweathermap_3.0", "read_error").Inc()
		return nil, err
	}

	var oneCall OneCallResponse
	if err := json.Unmarshal(body, &oneCall); err != nil {
		metrics.ExternalAPIFailuresTotal.WithLabelValues("openweathermap_3.0", "parse_error").Inc()
		return nil, err
	}

	// Convert One Call API response to our WeatherData format
	weatherData := &WeatherData{
		Main: struct {
			Temp     float64 `json:"temp"`
			Humidity int     `json:"humidity"`
		}{
			Temp:     oneCall.Current.Temp,
			Humidity: oneCall.Current.Humidity,
		},
		Wind: struct {
			Speed float64 `json:"speed"`
		}{
			Speed: oneCall.Current.WindSpeed,
		},
	}

	// Map weather conditions
	if len(oneCall.Current.Weather) > 0 {
		weatherData.Weather = []struct {
			Main string `json:"main"`
		}{
			{Main: oneCall.Current.Weather[0].Main},
		}
	}

	return weatherData, nil
}

