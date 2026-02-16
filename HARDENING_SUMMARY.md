# Production Hardening Reference

This document explains the production hardening measures implemented in the weather-service and serves as a reference for understanding the reliability and operational patterns used throughout the codebase.

---

## Error Handling & Retry Logic

### Structured Error Types

**Location:** `internal/weather/errors.go`

The service uses structured error types instead of string matching for reliable error classification:

```go
type APIError struct {
    StatusCode int
    Message    string
    Source     string  // "geocoding" or "weather"
}

func (e *APIError) IsRetryable() bool {
    return e.StatusCode == 429 || e.StatusCode >= 500
}
```

**Benefits:**
- Type-safe error checking eliminates fragile string matching
- Explicit retry policy based on HTTP status codes
- Network errors properly wrapped and always retryable
- Status codes and sources tracked for observability

**Retry Policy:**
- ✅ **Retryable:** 429 (rate limit), 5xx (server errors), network errors
- ❌ **Non-retryable:** 400, 401, 404 (client errors)

**Example Usage:**
```go
if apiErr, ok := lastErr.(*APIError); ok && !apiErr.IsRetryable() {
    // Don't retry - client error
    return nil, apiErr
}
// Otherwise, retry with backoff
```

### Exponential Backoff with Jitter

**Location:** `internal/weather/client.go` (retry loop)

Prevents thundering herd problem when multiple instances retry simultaneously:

```go
// Exponential backoff: 100ms, 200ms, 400ms
baseBackoff := time.Duration(math.Pow(2, float64(attempt-1))*100) * time.Millisecond

// Proportional jitter: ±25% of backoff
jitterRange := baseBackoff / 4
jitter := time.Duration(rand.Int63n(int64(jitterRange*2))) - jitterRange
time.Sleep(baseBackoff + jitter)
```

**Key Features:**
- RNG seeded at package init to ensure randomness across instances
- Jitter proportional to backoff (not fixed)
- Backoff/jitter values logged for debugging

**Max Retries:** 3 attempts (configurable)

---

## Configuration Validation

**Location:** `internal/config/config.go`

### Fail-Fast Validation

The service validates all configuration at startup and refuses to start with invalid config:

```go
func (c *Config) Validate() error {
    // API key required
    if c.WeatherAPIKey == "" {
        return errors.New("WEATHER_API_KEY must be set")
    }

    // Timeout relationship
    if c.UpstreamTimeoutSeconds >= c.RequestTimeoutSeconds {
        return fmt.Errorf("UPSTREAM_TIMEOUT_SECONDS (%d) must be less than REQUEST_TIMEOUT_SECONDS (%d)", ...)
    }

    // Port range
    if c.Port < 1 || c.Port > 65535 {
        return fmt.Errorf("PORT must be between 1 and 65535, got %d", c.Port)
    }

    // ... additional validations
}
```

### Validation Rules

| Configuration | Rule | Why |
|---------------|------|-----|
| `WEATHER_API_KEY` | Must be set | Required for API access |
| Timeouts | `UPSTREAM_TIMEOUT < REQUEST_TIMEOUT` | Prevents timeout race conditions |
| `PORT` | 1-65535 | Valid TCP port range |
| `LOG_LEVEL` | Valid zerolog level | Prevents runtime logging errors |
| Numeric configs | Must be positive (≥1) | Prevents invalid values |

**Usage:**
```go
cfg, err := config.Load()
if err != nil {
    log.Fatal().Err(err).Msg("invalid configuration")
}
```

---

## HTTP Client Configuration

**Location:** `internal/weather/client.go` (NewClient)

### Custom Transport

The HTTP client uses a custom transport for optimal connection pooling:

```go
httpClient: &http.Client{
    Timeout: time.Duration(cfg.UpstreamTimeoutSeconds) * time.Second,
    Transport: &http.Transport{
        MaxIdleConns:        100,  // Total idle connections
        MaxIdleConnsPerHost: 10,   // Per-host idle connections
        MaxConnsPerHost:     10,   // Max active connections per host
        IdleConnTimeout:     90 * time.Second,
        TLSHandshakeTimeout: 10 * time.Second,
        DisableKeepAlives:   false,
    },
}
```

**Benefits:**
- Connection reuse across requests (reduces latency)
- Prevents connection exhaustion under load
- Proper TLS handshake timeouts
- Keep-alive enabled for better performance

**Connection Pool Tuning:**
- Default limits are conservative (10 per host)
- Can be increased for high-throughput scenarios
- Monitor with Go runtime metrics: `go_goroutines`, `http_client_connections_*`

---

## Observability & Debugging

### Correlation IDs

**Location:** `internal/middleware/correlation_id.go`

Every request gets a unique correlation ID for tracing:

```go
correlationID := uuid.New().String()
ctx := context.WithValue(r.Context(), CorrelationIDKey, correlationID)
w.Header().Set("X-Correlation-ID", correlationID)
```

**Usage in Logs:**
```go
// Extract from context
if correlationID := ctx.Value(middleware.CorrelationIDKey); correlationID != nil {
    logger = logger.With().Str("correlation_id", correlationID.(string)).Logger()
}
logger.Error().Msg("request failed")
```

**Benefits:**
- Trace requests across retries, cache lookups, and upstream calls
- Returned in response header for client-side debugging
- Logged at every decision point (cache hit/miss, retry, error)

### Structured Logging

**Pattern:** All logs use structured zerolog fields:

```go
log.Info().
    Str("location", location).
    Str("api_version", version).
    Int("status_code", statusCode).
    Msg("weather data fetched")
```

**Key Log Events:**
- `cache hit/miss` - Cache decisions
- `retrying request` - Retry attempts with backoff/jitter
- `stale cache used` - Stale-on-error activations
- `load shedding activated` - Saturation events
- `upstream API error` - External API failures

---

## Resilience Patterns

### Stale-on-Error

**Location:** `internal/cache/cache.go`

Serves stale cached data when upstream fails, with age guard:

```go
// If fresh data fetch fails, serve stale if not too old
staleAge := time.Since(item.StoredAt)
if staleAge < c.ttl * 2 {  // Max 2x TTL (10 minutes for 5-min TTL)
    return item.Value, nil
}
return nil, ErrCacheTooStale
```

**Behavior:**
- Serves stale data up to 2× TTL (10 minutes for default 5-min TTL)
- Prevents serving arbitrarily old data
- Logged with `stale_cache_used` metric and log event

**When It Activates:**
- Upstream API returns 5xx errors after retries
- Network failures after retries
- Rate limit exceeded (429) after retries

### Load Shedding

**Location:** `internal/middleware/load_shed.go`

Rejects requests when inflight count exceeds threshold:

```go
if atomic.LoadInt64(&inflight) >= threshold {
    metrics.LoadShedTotal.Inc()
    http.Error(w, "Service overloaded", http.StatusServiceUnavailable)
    return
}
```

**Configuration:**
- Default threshold: 100 concurrent requests
- Returns 503 Service Unavailable
- Metric: `load_shed_total` (counter)
- Logged with correlation ID

**Tuning:**
- Set `LOAD_SHED_THRESHOLD` based on measured capacity
- Monitor `http_inflight_requests` gauge to establish baseline
- Consider HPA (Horizontal Pod Autoscaler) if shedding occurs frequently

### Rate Limiting

**Location:** `internal/middleware/rate_limit.go`

Per-IP token bucket rate limiting:

```go
limiter := rate.NewLimiter(rate.Limit(rps), rps)
if !limiter.Allow() {
    http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
    return
}
```

**Configuration:**
- Default: 50 requests/second per IP
- Set via `RATE_LIMIT_RPS` environment variable
- Returns 429 Too Many Requests
- Metric: `rate_limit_total` (counter)

**Production Considerations:**
- Consider distributed rate limiting (Redis) for multi-instance deployments
- Current implementation is per-instance, not cluster-wide
- May need adjustment based on upstream API limits (OpenWeather: 60 rps)

---

## Security

### Distroless Container

**Location:** `Dockerfile`

Uses minimal distroless base image:

```dockerfile
FROM gcr.io/distroless/static-debian12
USER 1000:1000
COPY --from=builder /app/weather-service /app/weather-service
```

**Security Features:**
- No shell or package manager (attack surface minimization)
- Runs as non-root user (UID 1000)
- Static binary (no libc dependencies/vulnerabilities)
- Read-only root filesystem (K8s security context)

### Kubernetes Security Context

**Location:** `deployments/kubernetes/base/deployment.yaml`

```yaml
securityContext:
  runAsNonRoot: true
  runAsUser: 1000
  readOnlyRootFilesystem: true
  allowPrivilegeEscalation: false
  capabilities:
    drop:
      - ALL
```

**Benefits:**
- Prevents privilege escalation
- Immutable runtime (no file modifications)
- Minimal Linux capabilities
- Defense-in-depth against container breakout

---

## Monitoring & Alerting

### Key Metrics

**Request Metrics:**
- `http_requests_total{status,method}` - Request count by status/method
- `http_request_duration_seconds{handler}` - Request latency histogram
- `http_inflight_requests` - Current concurrent requests

**Dependency Metrics:**
- `external_api_requests_total{api,version}` - Upstream API calls
- `external_api_failures_total{api,version}` - Upstream failures
- `cache_hits_total` - Cache effectiveness
- `cache_misses_total` - Cache misses

**Reliability Metrics:**
- `rate_limit_total` - Rate limit activations
- `load_shed_total` - Load shedding activations
- `stale_cache_used_total` - Stale-on-error activations

### Alert Rules

**Location:** `deployments/observability/alerts.yaml`

**HighErrorRate:**
```promql
rate(http_requests_total{status=~"5.."}[5m]) /
  clamp_min(rate(http_requests_total[5m]), 1) > 0.01
```
Triggers when error rate exceeds 1% (violates 99% SLO)

**HighLatency:**
```promql
histogram_quantile(0.95,
  rate(http_request_duration_seconds_bucket[5m])) > 0.5
```
Triggers when P95 latency exceeds 500ms

**LoadSheddingActive:**
```promql
rate(load_shed_total[5m]) > 0
```
Triggers if any load shedding occurs

---

## Resource Limits

**Location:** `deployments/kubernetes/base/deployment.yaml`

```yaml
resources:
  requests:
    cpu: 100m        # Guaranteed CPU
    memory: 128Mi    # Guaranteed memory
  limits:
    cpu: 500m        # CPU burst limit
    memory: 256Mi    # OOM kill threshold
```

**Rationale:**
- **Requests:** Minimum guaranteed resources for scheduling
- **Limits:** Prevent resource starvation of other pods
- **CPU:** 100m sufficient for steady state, 500m for burst traffic
- **Memory:** 128Mi typical, 256Mi includes cache + connections

**Tuning:**
- Monitor with `kubectl top pods`
- Increase if CPU throttling or OOM kills occur
- Consider HPA if load patterns are predictable

---

## Health Checks

### Liveness Probe

**Endpoint:** `GET /health`
**Purpose:** Determine if pod should be restarted

```yaml
livenessProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 10
  periodSeconds: 10
```

**Behavior:**
- Returns 200 if service is alive
- Does NOT check upstream connectivity (important!)
- K8s restarts pod if health check fails

**Why Not Check Upstream:**
- Upstream outage should not kill all pods
- Pods can serve stale cache during outages
- Conflating liveness + readiness causes cascading failures

### Readiness Probe

**Status:** Currently same as liveness (suboptimal)

**Recommended Production Enhancement:**
- Create separate `/ready` endpoint
- Check upstream API connectivity
- Remove from load balancer if upstream unreachable
- Keep pod alive to serve stale cache

---

## Configuration Reference

### Required

| Variable | Description | Example |
|----------|-------------|---------|
| `WEATHER_API_KEY` | OpenWeatherMap API key | `abc123...` |

### API Version

| Variable | Description | Default | Values |
|----------|-------------|---------|--------|
| `WEATHER_API_VERSION` | OpenWeather API version | `2.5` | `2.5`, `3.0` |

### Timeouts

| Variable | Description | Default | Range |
|----------|-------------|---------|-------|
| `REQUEST_TIMEOUT_SECONDS` | Total request timeout | 10s | 1-60s |
| `UPSTREAM_TIMEOUT_SECONDS` | Upstream API timeout | 5s | 1-30s |
| `CACHE_TTL_SECONDS` | Cache freshness window | 300s | 60-3600s |

**Timeout Relationship:** `UPSTREAM_TIMEOUT < REQUEST_TIMEOUT` (validated at startup)

### Rate Limiting & Load Shedding

| Variable | Description | Default | Range |
|----------|-------------|---------|-------|
| `RATE_LIMIT_RPS` | Requests per second per IP | 50 | 1-1000 |
| `LOAD_SHED_THRESHOLD` | Max concurrent requests | 100 | 10-10000 |

### Observability

| Variable | Description | Default | Values |
|----------|-------------|---------|--------|
| `LOG_LEVEL` | Logging verbosity | `info` | `debug`, `info`, `warn`, `error` |
| `PORT` | HTTP server port | 8080 | 1-65535 |

---

## Common Operational Patterns

### Handling Upstream Outages

**What Happens:**
1. Request comes in → cache miss
2. Upstream API call fails (network error or 5xx)
3. Retry 3 times with exponential backoff + jitter
4. All retries fail → check for stale cache
5. If stale cache exists and < 10 min old → serve stale
6. Otherwise → return 503 to client

**Metrics to Watch:**
- `stale_cache_used_total` - Increases during outage
- `external_api_failures_total{api="openweathermap"}` - Upstream failures
- `http_requests_total{status="503"}` - Failed requests (no stale cache available)

**Logs:**
```
correlation_id=abc123 msg="upstream API error" status_code=503
correlation_id=abc123 msg="serving stale cache data" age="8m30s"
```

### Handling Traffic Spikes

**What Happens:**
1. Traffic exceeds normal levels
2. Inflight requests reach `LOAD_SHED_THRESHOLD` (100)
3. Additional requests rejected with 503
4. `load_shed_total` metric increments

**Response:**
- Short-term: Increase `LOAD_SHED_THRESHOLD`
- Medium-term: Scale horizontally (add pods)
- Long-term: Implement HPA based on `http_inflight_requests`

### Debugging Slow Requests

**Metrics:**
```bash
# Check P95 latency
curl http://localhost:8080/metrics | grep http_request_duration_seconds

# Check cache hit rate
curl http://localhost:8080/metrics | grep cache_hits_total
curl http://localhost:8080/metrics | grep cache_misses_total
```

**Logs:**
```bash
# Follow logs for a specific correlation ID
kubectl logs -l app=weather-service | grep correlation_id=abc123
```

**Common Causes:**
- Cache misses forcing upstream calls (increase `CACHE_TTL_SECONDS`)
- Upstream API slow (check `external_api_requests_total` duration)
- Rate limiting / retry backoff (check for 429 responses)

---

## Not Implemented (Future Enhancements)

The following were considered but not implemented to avoid complexity:

1. **Circuit Breaker** - Would stop calling upstream after X consecutive failures
2. **Readiness Probe with Dependency Check** - Separate from liveness
3. **Request Context Deadline Propagation** - Carry parent timeouts through call chain
4. **Adaptive Retry Backoff** - Adjust based on upstream response patterns
5. **Redis Cache** - For horizontal scaling and cache persistence
6. **OpenTelemetry Tracing** - Distributed request tracing

---

## Testing

**Location:** `*_test.go` files

**Key Test Files:**
- `internal/weather/errors_test.go` - Error type classification
- `internal/config/config_test.go` - Configuration validation
- `internal/cache/cache_test.go` - Cache operations including stale-on-error
- `internal/weather/client_test.go` - Client logic with mocked HTTP responses

**Run Tests:**
```bash
go test ./...
go test -v -race ./...  # With race detector
go test -coverprofile=coverage.out ./...  # With coverage
```

---

## Summary

The weather-service implements production-grade hardening through:

✅ **Type-safe error handling** with structured errors and explicit retry policies
✅ **Fail-fast configuration validation** at startup
✅ **Optimized HTTP client** with connection pooling
✅ **Proper retry jitter** to prevent thundering herd
✅ **Stale-on-error resilience** for upstream outages
✅ **Load shedding** for overload protection
✅ **Comprehensive observability** with metrics, logs, and correlation IDs
✅ **Security hardening** with distroless containers and minimal privileges

All patterns are implemented without external dependencies, keeping operational complexity minimal while achieving production reliability.
