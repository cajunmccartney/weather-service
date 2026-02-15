package cache

import (
	"fmt"
	"sync"
	"time"

	"weather-service/internal/metrics"

	"github.com/rs/zerolog/log"
)

type CacheEntry struct {
	Value     interface{}
	Timestamp time.Time
}

type Cache struct {
	data  map[string]*CacheEntry
	mu    sync.RWMutex
	ttl   time.Duration
}

func NewCache(ttl time.Duration) *Cache {
	c := &Cache{
		data: make(map[string]*CacheEntry),
		ttl:  ttl,
	}

	// Background cleanup
	go c.cleanup()

	return c
}

// Get retrieves from cache
func (c *Cache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.data[key]
	if !exists {
		metrics.CacheMissesTotal.WithLabelValues(key).Inc()
		return nil, false
	}

	// Check if expired
	if time.Since(entry.Timestamp) > c.ttl {
		metrics.CacheMissesTotal.WithLabelValues(key).Inc()
		return nil, false
	}

	metrics.CacheHitsTotal.WithLabelValues(key).Inc()
	return entry.Value, true
}

// v3.1 Enhancement: GetWithStale implements stale-on-error with age guard
func (c *Cache) GetWithStale(key string, fetchFn func() (interface{}, error)) (interface{}, error) {
	c.mu.RLock()
	entry, exists := c.data[key]
	c.mu.RUnlock()

	// Cache miss - fetch fresh
	if !exists {
		metrics.CacheMissesTotal.WithLabelValues(key).Inc()
		fresh, err := fetchFn()
		if err != nil {
			return nil, err
		}
		c.Set(key, fresh)
		return fresh, nil
	}

	age := time.Since(entry.Timestamp)

	// Fresh entry
	if age <= c.ttl {
		metrics.CacheHitsTotal.WithLabelValues(key).Inc()
		return entry.Value, nil
	}

	// Expired - try to fetch fresh
	fresh, err := fetchFn()
	if err != nil {
		// v3.1 Enhancement: Fetch failed - check if stale is acceptable
		maxStaleWindow := c.ttl * 2

		if age < maxStaleWindow {
			// Serve stale value
			metrics.CacheStaleServedTotal.Inc()
			log.Warn().
				Str("key", key).
				Dur("stale_age", age).
				Dur("max_window", maxStaleWindow).
				Msg("serving stale cache entry on upstream failure")
			return entry.Value, nil
		}

		// Too stale - return error
		return nil, fmt.Errorf("cache too stale (age: %v, max: %v): %w", age, maxStaleWindow, err)
	}

	// Fresh fetch succeeded
	c.Set(key, fresh)
	return fresh, nil
}

// Set adds to cache
func (c *Cache) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data[key] = &CacheEntry{
		Value:     value,
		Timestamp: time.Now(),
	}

	// Update size metric (rough estimate)
	metrics.CacheSizeBytes.Set(float64(len(c.data) * 1024)) // Rough estimate
}

// cleanup periodically removes expired entries
func (c *Cache) cleanup() {
	ticker := time.NewTicker(c.ttl)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for key, entry := range c.data {
			if now.Sub(entry.Timestamp) > c.ttl*3 { // Clean up very old entries
				delete(c.data, key)
				metrics.CacheEvictionsTotal.Inc()
			}
		}
		c.mu.Unlock()
	}
}
