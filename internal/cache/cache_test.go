package cache

import (
	"errors"
	"testing"
	"time"
)

func TestCacheHitMiss(t *testing.T) {
	cache := NewCache(1 * time.Second)

	// Miss
	_, found := cache.Get("key1")
	if found {
		t.Error("expected cache miss")
	}

	// Set and hit
	cache.Set("key1", "value1")
	val, found := cache.Get("key1")
	if !found {
		t.Error("expected cache hit")
	}
	if val != "value1" {
		t.Errorf("expected 'value1', got %v", val)
	}

	// Expiration
	time.Sleep(1100 * time.Millisecond)
	_, found = cache.Get("key1")
	if found {
		t.Error("expected cache miss after TTL")
	}
}

func TestStaleOnError(t *testing.T) {
	cache := NewCache(100 * time.Millisecond)

	fetchAttempts := 0
	fetchFn := func() (interface{}, error) {
		fetchAttempts++
		if fetchAttempts == 1 {
			return "fresh", nil
		}
		return nil, errors.New("upstream failure")
	}

	// First call - fresh
	val, err := cache.GetWithStale("key1", fetchFn)
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if val != "fresh" {
		t.Errorf("expected 'fresh', got %v", val)
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Second call - should serve stale on error
	val, err = cache.GetWithStale("key1", fetchFn)
	if err != nil {
		t.Fatalf("stale-on-error failed: %v", err)
	}
	if val != "fresh" {
		t.Errorf("expected stale 'fresh', got %v", val)
	}

	// Wait for stale window to expire (TTL * 2 = 200ms total)
	time.Sleep(100 * time.Millisecond)

	// Third call - stale too old, should return error
	_, err = cache.GetWithStale("key1", fetchFn)
	if err == nil {
		t.Error("expected error when cache too stale")
	}
}
