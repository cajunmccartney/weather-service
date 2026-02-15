package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"weather-service/internal/cache"
	"weather-service/internal/config"
	"weather-service/internal/middleware"
	"weather-service/internal/weather"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

var (
	weatherClient *weather.Client
	weatherCache  *cache.Cache
)

func InitWeather(cfg *config.Config) {
	weatherClient = weather.NewClient(cfg)
	weatherCache = cache.NewCache(time.Duration(cfg.CacheTTLSeconds) * time.Second)
}

type WeatherResponse struct {
	Location    string  `json:"location"`
	Temperature float64 `json:"temperature"`
	Conditions  string  `json:"conditions"`
	Humidity    int     `json:"humidity"`
	WindSpeed   float64 `json:"wind_speed"`
}

func GetWeather(w http.ResponseWriter, r *http.Request) {
	location := chi.URLParam(r, "location")

	// Get correlation ID and logger from context
	correlationID, _ := r.Context().Value(middleware.CorrelationIDKey).(string)
	logger := log.With().
		Str("correlation_id", correlationID).
		Str("location", location).
		Logger()

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Try cache with stale-on-error fallback
	cacheKey := "weather:" + location
	data, err := weatherCache.GetWithStale(cacheKey, func() (interface{}, error) {
		return weatherClient.FetchWeather(ctx, location)
	})

	if err != nil {
		// LOG THE ERROR - this is critical for debugging
		logger.Error().Err(err).Msg("failed to fetch weather data")
		http.Error(w, "failed to fetch weather data", http.StatusServiceUnavailable)
		return
	}

	weatherData := data.(*weather.WeatherData)

	// Check for nil data
	if weatherData == nil {
		logger.Error().Msg("weather data is nil")
		http.Error(w, "invalid weather data", http.StatusInternalServerError)
		return
	}

	// Check for empty weather conditions
	if len(weatherData.Weather) == 0 {
		logger.Error().Msg("weather conditions array is empty")
		http.Error(w, "incomplete weather data", http.StatusInternalServerError)
		return
	}

	response := WeatherResponse{
		Location:    location,
		Temperature: weatherData.Main.Temp,
		Conditions:  weatherData.Weather[0].Main,
		Humidity:    weatherData.Main.Humidity,
		WindSpeed:   weatherData.Wind.Speed,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
