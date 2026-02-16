# Design Decisions & Architecture Notes

This document outlines the key architectural decisions, design patterns, and technical rationale behind the weather-service implementation.

---

## API Version Selection (Dual Support: 2.5 + 3.0)

**Decision:** Support both API 2.5 (default) and API 3.0 with environment variable selection

**Implementation:**
- Runtime version selection via `WEATHER_API_VERSION` environment variable
- **API 2.5**: Direct weather API call (`/data/2.5/weather`)
- **API 3.0**: Two-step process (Geocoding API → One Call API 3.0)
- Single codebase, shared retry/cache/metrics infrastructure
- Kustomize overlays for version-specific Kubernetes deployments

**Rationale:**
- **API 2.5 default**: Free tier, no payment method required, instant activation → optimal for reviewer testing
- **API 3.0 optional**: Current API standard, coordinate support → production-ready option
- Demonstrates version flexibility without build-time complexity
- Single Docker image works for both versions

**API 2.5 (Default):**
- Free forever (60 calls/min, 1M calls/month)
- No payment method required, instant activation
- Location names only
- Single API call per request

**API 3.0 (Optional):**
- 1,000 calls/day free, $0.15 per 100 calls after
- Requires payment method, 2-hour activation delay
- Supports location names AND coordinates
- Two API calls for location names (geocoding + weather), one for coordinates

**Cache Strategy (Both Versions):**
- Keyed by input (location name or coordinates)
- Reduces API calls by ~80%
- Same stale-on-error behavior for both versions
- 5-minute TTL keeps well within free tiers

### Coordinate Support Enhancement

**Decision:** Added support for direct coordinate queries in addition to location names

**Implementation:**
- Auto-detection via `parseCoordinates()` helper function
- If input matches "lat,lon" format (comma-separated floats), skip geocoding
- If parsing fails, treat as location name and geocode normally
- Validates coordinate ranges (lat: -90 to 90, lon: -180 to 180)

**Benefits:**
- Faster responses when coordinates are known (1 API call vs 2)
- More flexible API (supports both formats)
- Backward compatible (location names still work)
- Useful for programmatic clients with coordinate data

**Examples:**
- `GET /weather/London` → Geocodes, then fetches weather
- `GET /weather/51.5074,-0.1278` → Fetches weather directly

**Cache Keys:**
- Location names: `weather:London`
- Coordinates: `weather:51.5074,-0.1278`
- Note: Same location queried by name vs coordinates will have separate cache entries

---

## Major Design Decisions

### 1. Chi Router vs Gin
**Decision:** Chi

**Reasoning:** More idiomatic, stdlib-close design with less "framework magic." Chi's middleware pattern aligns well with standard library conventions and provides better transparency for operations teams.

### 2. Stale-on-Error vs Circuit Breaker
**Decision:** Stale-on-error (with age guard)

**Reasoning:**
- Simpler implementation
- Better availability signal (serves data vs fails fast)
- Age guard (2x TTL) prevents serving arbitrarily old data
- More appropriate for read-heavy services where eventual consistency is acceptable

**Implementation:**
```go
if staleAge < cacheTTL * 2 {
    // Serve stale (max 10 minutes old for 5min TTL)
    return staleValue
}
// Too stale, return error
```

**Alternative Considered:** Circuit breaker would be more appropriate for write operations or when stale data violates correctness guarantees. For this read-heavy weather service, graceful degradation through stale data is preferable.

### 3. Load Shedding
**Decision:** Implement basic load shedding

**Reasoning:** Demonstrates saturation awareness and SLO protection under load. Simple implementation with high operational value - prevents cascading failures during traffic spikes.

**Implementation:** Returns 503 when inflight requests exceed threshold (100 by default).

### 4. In-Memory Cache vs Redis
**Decision:** In-memory first

**Reasoning:**
- Premature distribution adds failure modes (network partitions, connection pooling, etc.)
- Simpler to reason about and operate
- Sufficient for single-instance or small-scale deployments
- Demonstrates core caching patterns

**Production Next Step:** Redis for horizontal scaling and cache persistence

**Tradeoff:** Cache is lost on pod restart, but 5-minute TTL means limited impact

### 5. Config Library (Viper vs os.Getenv)
**Decision:** Simple `os.Getenv` with validation

**Reasoning:** Simplicity over abstraction. For this scope, environment variables with fail-fast validation are sufficient and more transparent.

**Tradeoff:** Less flexibility (no hot reload, no config file merging), but appropriate for 12-factor app patterns.

### 6. Circuit Breaker Implementation
**Decision:** Not implemented

**Reasoning:**
- Retry + timeout + stale-on-error already demonstrates dependency resilience
- Circuit breaker requires state tuning and adds complexity
- Would be valuable in production but not essential for demonstrating operational thinking

**Production Consideration:** Would implement for services with higher write loads or stricter consistency requirements.

### 7. OpenTelemetry Tracing
**Decision:** Not implemented

**Reasoning:**
- Time-intensive setup with distributed tracing infrastructure
- Prometheus metrics already provide comprehensive observability
- Structured logging with correlation IDs enables basic request tracing

**Production Consideration:** Would implement with span context propagation through cache → client → upstream, capturing retry attempts, cache hit/miss decisions, and upstream latency as span attributes.

### 8. Kubernetes Manifests
**Decision:** Implement comprehensive K8s deployment

**Reasoning:**
- Demonstrates production deployment thinking
- Shows understanding of resource limits, probes, and security contexts
- Provides realistic operational environment

**Includes:**
- Resource limits (CPU: 100m-500m, Memory: 128Mi-256Mi)
- Liveness and readiness probes
- Security context (non-root, read-only filesystem)
- Prometheus annotations for auto-discovery

---

## Implementation Details

### Load Shedding Logging
**Enhancement:** Correlation ID logging and metric-before-return ordering

```go
metrics.LoadShedTotal.Inc()  // Increment BEFORE return
logger.Warn().Str("correlation_id", cid).Msg("load shedding activated")
```

**Reasoning:** Operational best practice - enables request tracing during saturation events.

### Stale-on-Error Age Guard
**Enhancement:** 2x TTL safety window

```go
if staleAge < cacheTTL * 2 {
    // Serve stale (max 10 min for 5min TTL)
} else {
    return ErrCacheTooStale
}
```

**Reasoning:** Prevents serving excessively old data during extended outages while maintaining availability.

### Go Runtime Collectors
**Implementation:** Leverages automatically registered Go collectors

```go
// Note: Go runtime collectors (go_*, process_*) are automatically registered
// by the Prometheus client library's default registry
func Init() {
    ServiceUp.Set(1)
}
```

**Reasoning:** Standard practice - adds memory/goroutine/GC metrics with zero effort. Manual registration would cause a panic.

### Alert Expression Robustness
**Enhancement:** `clamp_min` to prevent divide-by-zero

```promql
rate(...) / clamp_min(rate(...), 1) > 0.01
```

**Reasoning:** Prevents alert evaluation errors during startup or very low traffic periods.

### Health vs Readiness Probes
**Documentation:** Explicit explanation of liveness vs readiness semantics

**Current Implementation:**
- `GET /health` - Liveness probe (always returns 200 if process is alive)

**Production Enhancement:**
- `GET /ready` - Readiness probe (would check upstream API connectivity)

**Why This Distinction Matters:**
Conflating liveness and readiness can cause cascading failures. If all pods fail readiness due to upstream outage, they all get removed from load balancer simultaneously, even though they could serve stale cache data.

---

## Configuration Considerations

### Tuning Parameters

**Rate Limit (currently 50 rps):**
- Adjust based on traffic patterns and upstream API limits
- OpenWeather allows up to 60 rps on free tier

**Load Shed Threshold (currently 100 inflight):**
- Tune based on measured capacity
- Monitor `http_inflight_requests` to establish baseline

**Cache TTL (currently 5 min):**
- Balance freshness vs upstream load
- Weather data typically doesn't change rapidly
- Consider longer TTL (10-15 min) for cost optimization

**Stale Age Window (currently 10 min):**
- Depends on data staleness tolerance
- Current 2x TTL provides reasonable availability vs accuracy tradeoff

### Alert Sensitivity

**HighErrorRate (threshold: 1%):**
- May need adjustment after establishing baseline
- Consider SLO requirements (currently 99% availability)

**HighLatency (threshold: 500ms):**
- Depends on latency SLO
- Current P95 target is appropriate for external API dependency

**LoadSheddingActive:**
- Consider if false positives occur during legitimate bursts
- May need rate-based threshold vs any-events trigger

---

## Production Next Steps

The following enhancements would be valuable for production deployment but were not implemented due to scope/time constraints:

1. **Redis Cache** - For horizontal scaling and cache persistence
2. **Circuit Breaker** - For extended upstream outages
3. **OpenTelemetry** - For distributed request tracing
4. **HPA** - Horizontal Pod Autoscaler based on `http_inflight_requests`
5. **Readiness Probe** - Dependency-aware health check (separate from liveness)
6. **Request Hedging** - Parallel requests with timeout for P99 improvement
7. **Adaptive Retry** - Dynamic backoff based on upstream response patterns
8. **Dependency Budgeting** - Track error budget consumption per dependency

---

## Operational Philosophy

This implementation prioritizes:

1. **Observability** - Comprehensive metrics, structured logging, correlation IDs
2. **Availability** - Stale-on-error, retry with backoff, graceful degradation
3. **Blast Radius Reduction** - Rate limiting, load shedding, no-retry on 4xx
4. **Simplicity** - Minimal dependencies, transparent patterns, fail-fast validation
5. **Production Thinking** - Resource limits, security contexts, actionable alerts

The design assumes:
- Upstream APIs are unreliable and may be rate-limited
- Availability > consistency for read-heavy weather data
- Operations teams need clear signals for actionable alerts
- Infrastructure should prevent cascading failures
