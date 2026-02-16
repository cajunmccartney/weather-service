
Production-ready backend service demonstrating observability, reliability, and operational excellence.

## Architecture Overview

```
Client → Chi Router → Middleware Stack → Weather Handler → Cache → Weather API Client → OpenWeatherMap
```

**Key Components:**
- **Router**: Chi (idiomatic, lightweight)
- **Cache**: In-memory with stale-on-error fallback
- **Retry**: Exponential backoff with jitter (max 3 attempts)
- **Rate Limiting**: Token bucket (50 rps)
- **Load Shedding**: Inflight request tracking (threshold: 100)
- **Observability**: 16 Prometheus metrics + Go runtime metrics

## SLO Definitions

```
Availability SLO: 99% of /weather requests return 2xx
Latency SLO: 95% of requests complete under 500ms
Dependency SLO: 99.9% availability over 30 days
```

## API Endpoints

### `GET /weather/{location}`
Returns current weather data for the specified location.

**Input formats (depends on API version):**

**API 2.5 (default):**
- Location names: `London`, `Tokyo`, `San Francisco` (URL encode spaces)
- City with country: `London,GB`, `Paris,FR`
- ZIP codes: `10001,US`

**API 3.0 (if enabled):**
- All 2.5 formats above, plus:
- Coordinates: `51.5074,-0.1278`, `40.7128,-74.0060`

**Examples:**
```bash
# API 2.5 (default)
curl http://localhost:8080/weather/London
curl http://localhost:8080/weather/San%20Francisco

# API 3.0 (set WEATHER_API_VERSION=3.0)
curl http://localhost:8080/weather/London
curl http://localhost:8080/weather/51.5074,-0.1278
```

**Response:**
```json
{
  "location": "London",
  "temperature": 38.59,
  "conditions": "Clouds",
  "humidity": 78,
  "wind_speed": 8.99
}
```

**How it works:**
- **API 2.5:** Direct weather API call (1 request)
- **API 3.0 with location names:** Geocodes, then fetches weather (2 requests)
- **API 3.0 with coordinates:** Direct weather API call (1 request, faster)

**Caching:** 5-minute TTL with stale-on-error fallback (max 10 minutes)

### `GET /health`
Returns service liveness status.

**Note:** This is a **liveness probe**, not readiness. It returns 200 if the process is alive, without checking dependencies. See "Health vs. Readiness" section below.

### `GET /metrics`
Exposes Prometheus-compatible metrics.

## Configuration

All configuration via environment variables. See [HARDENING_SUMMARY.md](HARDENING_SUMMARY.md#configuration-reference) for detailed configuration reference.

| Variable | Default | Description |
|----------|---------|-------------|
| `WEATHER_API_KEY` | *(required)* | OpenWeatherMap API key |
| `WEATHER_API_VERSION` | `2.5` | API version: `2.5` or `3.0` |
| `PORT` | `8080` | HTTP server port |
| `LOG_LEVEL` | `info` | Log verbosity: `debug`, `info`, `warn`, `error` |
| `CACHE_TTL_SECONDS` | `300` | Cache freshness (5 min) |
| `REQUEST_TIMEOUT_SECONDS` | `10` | Total request timeout |
| `UPSTREAM_TIMEOUT_SECONDS` | `5` | Upstream API timeout (must be < REQUEST_TIMEOUT) |
| `RATE_LIMIT_RPS` | `50` | Requests per second per IP |
| `LOAD_SHED_THRESHOLD` | `100` | Max concurrent requests before shedding |

## API Version Overview

This service supports two OpenWeather API versions. **Choose 2.5 for quick testing** (no payment method required), or 3.0 for the current API with coordinate support.

| Feature | API 2.5 (Default) | API 3.0 |
|---------|-------------------|---------|
| **Status** | Deprecated but functional | Current version |
| **Payment Method** | Not required | Required on file |
| **Activation** | Instant | 2-hour delay for new accounts |
| **Free Tier** | 60 calls/min, 1M calls/month | 1,000 calls/day |
| **Overage Cost** | N/A (free forever) | $0.15 per 100 calls |
| **Query Support** | Location names only | Location names + coordinates |
| **Geocoding** | Not used | Free (automatic) |

**Cost Optimization (both versions):**
- 5-minute cache reduces API calls by ~80%
- Stale-on-error serves cached data during outages
- Rate limiting prevents abuse

## Deployment

**Recommended:** Kubernetes deployment below. For Docker-only or local development, see [Alternative Deployment Methods](#alternative-deployment-methods).

### Kubernetes/Minikube Deployment

Production-ready deployment with health checks, resource limits, and observability. Supports both API versions via Kustomize overlays.

```bash
# 1. Build and load image into Minikube
eval $(minikube docker-env)
docker build -t weather-service:latest .

# 2. Create secret with your OpenWeatherMap API key
./deployments/kubernetes/create-secret.sh your_openweathermap_api_key

# 3. Deploy with your chosen API version
#    API 2.5 (default, free, no payment method):
kubectl apply -k deployments/kubernetes/overlays/api-2.5

#    API 3.0 (requires payment method, supports coordinates):
kubectl apply -k deployments/kubernetes/overlays/api-3.0

# 4. Verify deployment
kubectl get pods -l app=weather-service
kubectl rollout status deployment/weather-service

# 5. Access the service (in a separate terminal)
kubectl port-forward svc/weather-service 8080:80

# 6. Test endpoints (in another terminal)
curl http://localhost:8080/health
curl http://localhost:8080/weather/London
curl http://localhost:8080/metrics
```

**Features:**
- Resource limits (CPU: 100m-500m, Memory: 128Mi-256Mi)
- Liveness and readiness probes
- Security context (non-root, read-only filesystem)
- Prometheus annotations for auto-discovery
- Clean version switching with Kustomize overlays

#### Operations

**Switch API versions:**
```bash
# Switch to 3.0
kubectl apply -k deployments/kubernetes/overlays/api-3.0

# Switch to 2.5
kubectl apply -k deployments/kubernetes/overlays/api-2.5

# Verify
kubectl rollout status deployment/weather-service
```

**View logs:**
```bash
kubectl logs -l app=weather-service --tail=100 -f
```

**Update API key:**
```bash
kubectl delete secret weather-api-secret
./deployments/kubernetes/create-secret.sh new_api_key
kubectl rollout restart deployment/weather-service
```

**Monitor resource usage:**
```bash
kubectl top pods -l app=weather-service
```

**Check deployment status:**
```bash
kubectl describe deployment weather-service
kubectl get events --sort-by=.metadata.creationTimestamp
```

See [deployments/kubernetes/README.md](deployments/kubernetes/README.md) for Kustomize details.

---

### Alternative Deployment Methods

<details>
<summary><b>Docker (Standalone)</b> - Click to expand</summary>

Quick local testing without Kubernetes orchestration.

#### Run with Docker

```bash
# Build image
docker build -t weather-service:latest .

# Run with API 2.5 (free, no payment method)
docker run -d -p 8080:8080 \
  --name weather-service \
  -e WEATHER_API_KEY=your_key_here \
  -e WEATHER_API_VERSION=2.5 \
  weather-service:latest

# Run with API 3.0 (requires payment method, supports coordinates)
docker run -d -p 8080:8080 \
  --name weather-service \
  -e WEATHER_API_KEY=your_key_here \
  -e WEATHER_API_VERSION=3.0 \
  weather-service:latest

# Test endpoints
curl http://localhost:8080/health
curl http://localhost:8080/weather/London
curl http://localhost:8080/metrics
```

#### Operations

**View logs:**
```bash
docker logs -f weather-service
```

**Restart container:**
```bash
docker restart weather-service
```

**Stop and remove:**
```bash
docker stop weather-service
docker rm weather-service
```

**Update configuration:**
```bash
# Stop existing container
docker stop weather-service && docker rm weather-service

# Run with new config
docker run -d -p 8080:8080 \
  --name weather-service \
  -e WEATHER_API_KEY=new_key \
  -e WEATHER_API_VERSION=3.0 \
  weather-service:latest
```

**Check resource usage:**
```bash
docker stats weather-service
```

</details>

<details>
<summary><b>Native Go (Development)</b> - Click to expand</summary>

For active development without containerization.

#### Run Locally

```bash
# Build and test
make build
make test

# Run with API 2.5 (free, no payment method)
export WEATHER_API_KEY=your_key_here
export WEATHER_API_VERSION=2.5
make run

# Run with API 3.0 (requires payment method, supports coordinates)
export WEATHER_API_KEY=your_key_here
export WEATHER_API_VERSION=3.0
make run

# Test endpoints
curl http://localhost:8080/health
curl http://localhost:8080/weather/London
curl http://localhost:8080/metrics
```

#### Operations

**Run with debug logging:**
```bash
export LOG_LEVEL=debug
make run
```

**Run tests with coverage:**
```bash
make test
# Or with detailed coverage report:
go test -v -race -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

**Build for production:**
```bash
CGO_ENABLED=0 go build -ldflags="-w -s" -o bin/weather-service cmd/server/main.go
```

**Clean build artifacts:**
```bash
make clean
```

</details>

### Observability Stack
```bash
# Deploy Prometheus and AlertManager
kubectl apply -f deployments/observability/
```

## Failure Modes & Mitigations

| Failure Mode | Impact | Mitigation | Observable Signal |
|--------------|--------|------------|-------------------|
| **Upstream timeout** | Request fails | Retry with exponential backoff (max 3) + stale cache fallback | `external_api_failures_total{error_type="timeout"}` |
| **Upstream rate limit** | 429 from API | Rate limiting + cache (reduces upstream calls) | `external_api_failures_total{error_type="4xx"}` |
| **Upstream extended outage** | All requests fail | Stale-on-error cache (serves data up to 10min old) | `cache_stale_served_total` |
| **Memory growth** | OOMKill | Cache eviction + K8s memory limits | `cache_size_bytes`, `process_resident_memory_bytes` |
| **High RPS burst** | Service saturation | Load shedding (503 when >100 inflight) | `load_shed_total`, `http_inflight_requests` |
| **Client abuse** | Service overload | Rate limiting (429 when >50 rps) | `rate_limit_exceeded_total` |
| **Goroutine leak** | Memory/CPU exhaustion | Monitored via Go collectors | `go_goroutines` |
| **Location not found** | 404 from geocoding | Returns error to user | None (expected for invalid locations) |

## Design Decisions & Tradeoffs

### Why In-Memory Cache vs. Redis?
**Decision:** In-memory cache (Phase 1 implementation)

**Reasoning:**
- Simplicity: No additional infrastructure dependency
- Lower latency: No network hop
- Sufficient for single-instance or small-scale deployments
- Demonstrates core caching patterns

**Production Next Step:** Redis for horizontal scaling and cache persistence

**Tradeoff:** Cache is lost on pod restart, but 5-minute TTL means limited impact

### Why Chi Router vs. Gin?
**Decision:** Chi

**Reasoning:**
- More idiomatic (closer to stdlib `net/http`)
- Lightweight, no "framework magic"
- Better aligns with infrastructure-focused operational practices

**Tradeoff:** Slightly more verbose than Gin, but more transparent

### Why Stale-on-Error vs. Circuit Breaker?
**Decision:** Stale-on-error with 2x TTL safety guard

**Reasoning:**
- Better availability signal (serves data vs. fails fast)
- Simpler implementation (15 min vs. 30+ min for circuit breaker)
- Demonstrates graceful degradation thinking
- Circuit breaker is better for write operations or when stale data is unacceptable

**Implementation:**
```go
if cacheAge < TTL * 2 {
    // Serve stale (max 10 minutes old for 5min TTL)
    return staleValue
}
// Too stale, return error
```

### Why Simple Config (os.Getenv) vs. Viper?
**Decision:** Simple `os.Getenv` with validation

**Reasoning:**
- Config complexity not needed for this service
- Simplicity over abstraction
- Fail-fast validation at startup
- No external dependencies

**Tradeoff:** Less flexibility (no hot reload, no config file merging), but appropriate for scope

### Why No OpenTelemetry Implementation?
**Decision:** Not implemented

**Reasoning:**
- Time-intensive (setup, exporter debugging)
- Prometheus metrics already satisfy observability requirements
- Tracing is valuable but not essential for current scope

**Production Consideration:**
> In production, I would instrument with OpenTelemetry and export to an OTLP collector for distributed tracing. I'd trace request paths through cache → weather client → upstream API with span attributes for cache hit/miss, retry attempts, and upstream latency.

### Health vs. Readiness Probes

**Current Implementation:**
- `GET /health` - **Liveness probe**
  - Always returns 200 if process is alive
  - Used by K8s to restart crashed pods
  - Does NOT check dependencies

**Production Next Step:**
- `GET /ready` - **Readiness probe** (not implemented due to time)
  - Would check upstream API connectivity (with timeout)
  - Would validate cache availability
  - Used by K8s to control traffic routing

**Why This Distinction Matters:**

Conflating liveness and readiness can cause cascading failures:
1. Upstream API has an outage
2. All pods fail readiness due to dependency check
3. All pods removed from load balancer simultaneously
4. Service goes completely down even though it could serve stale cache

**Correct Behavior:**
- Liveness: Simple (process alive?)
- Readiness: Dependency-aware (can I serve traffic?)

This separation allows the service to continue serving stale cached data during upstream outages while preventing new pod deployments from accepting traffic until dependencies are healthy.

## Alerting Strategy

### What Each Alert Protects Against

**HighErrorRate**
- **Protects:** Availability SLO
- **Trigger:** >1% error rate for 2 minutes
- **Action:** Page on-call
- **Common Causes:** Upstream API failures, application bugs, configuration errors

**HighLatency**
- **Protects:** Latency SLO
- **Trigger:** P95 >500ms for 5 minutes
- **Action:** Warning (investigate)
- **Common Causes:** Upstream API slowness, low cache hit rate, resource constraints

**UpstreamAPIFailures**
- **Protects:** Dependency health
- **Trigger:** >0.1 failures/sec for 2 minutes
- **Action:** Page on-call
- **Common Causes:** OpenWeatherMap outage, network issues, API key problems

**LoadSheddingActive**
- **Protects:** Service stability under saturation
- **Trigger:** Any load shedding events
- **Action:** Warning (scale horizontally)
- **Common Causes:** Traffic spike, insufficient capacity, slow requests

**LowCacheHitRate**
- **Protects:** Upstream API from excessive load
- **Trigger:** <50% hit rate for 10 minutes
- **Action:** Warning (investigate)
- **Common Causes:** Too-short TTL, traffic pattern change, cache eviction

## Testing

### Unit Tests
```bash
go test -v ./...
```

**Coverage:** >70% on critical paths
- Retry logic (5xx vs 4xx behavior)
- Cache hit/miss/stale-on-error
- Rate limiting enforcement
- Context timeout propagation

### Integration Tests
```bash
# Start service
make run

# Basic functionality
curl http://localhost:8080/weather/London

# Rate limiting
ab -n 1000 -c 20 http://localhost:8080/weather/London

# Load shedding
ab -n 10000 -c 200 http://localhost:8080/weather/London

# Metrics validation
curl http://localhost:8080/metrics | grep weather
```

## Metrics Reference

### HTTP Layer
- `http_requests_total{method, route, status}` - Request counter
- `http_request_duration_seconds_bucket{route}` - Latency histogram
- `http_inflight_requests{route}` - Current active requests

### External API
- `external_api_requests_total{endpoint, status}` - Upstream request counter
- `external_api_duration_seconds_bucket{endpoint}` - Upstream latency
- `external_api_failures_total{endpoint, error_type}` - Failure types
- `external_api_retries_total` - Retry attempts

### Cache
- `cache_hits_total{location}` - Cache hits
- `cache_misses_total{location}` - Cache misses
- `cache_size_bytes` - Current cache size
- `cache_evictions_total` - Cache evictions
- `cache_stale_served_total` - Stale-on-error fallback uses

### Reliability
- `rate_limit_exceeded_total{client_id}` - Rate limit violations
- `load_shed_total` - Load shedding events

### Service Health
- `service_up` - Service availability (1=up, 0=down)
- `go_goroutines` - Current goroutine count
- `process_resident_memory_bytes` - Memory usage

## Production Considerations Not Implemented (Time Constraints)

1. **Redis Cache** - For horizontal scaling
2. **Circuit Breaker** - For extended outages (stale-on-error is Phase 1)
3. **OpenTelemetry Tracing** - For distributed request tracing
4. **HPA (Horizontal Pod Autoscaler)** - Based on `http_inflight_requests`
5. **Request Prioritization** - Shed low-priority requests first
6. **Background Cache Refresh** - Proactive cache warming
7. **Multi-region Deployment** - For disaster recovery
8. **Readiness Probe** - Dependency-aware health check

## License

MIT
