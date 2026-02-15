package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"weather-service/internal/config"
	"weather-service/internal/handlers"
	"weather-service/internal/metrics"
	"weather-service/internal/middleware"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("invalid configuration")
	}

	// Initialize logger
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
	if cfg.LogLevel == "debug" {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	// Initialize metrics (Phase 2)
	metrics.Init()

	// Initialize weather handler (Phase 5)
	handlers.InitWeather(cfg)

	// Initialize router
	r := chi.NewRouter()

	// Middleware (Phase 3)
	r.Use(middleware.CorrelationID)
	r.Use(middleware.Logger(logger))
	r.Use(middleware.Metrics)
	r.Use(middleware.RateLimit(cfg.RateLimitRPS))
	r.Use(middleware.LoadShedding(cfg.LoadShedThreshold))

	// Routes
	r.Get("/weather/{location}", handlers.GetWeather)
	r.Get("/health", handlers.Health)
	r.Get("/metrics", handlers.Metrics)

	// HTTP Server
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Graceful shutdown (Phase 7)
	go func() {
		logger.Info().Str("port", cfg.Port).Msg("server starting")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("server failed")
		}
	}()

	// Wait for interrupt
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info().Msg("shutting down server")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal().Err(err).Msg("server forced to shutdown")
	}

	logger.Info().Msg("server exited")
}
