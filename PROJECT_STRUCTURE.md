# Project Structure Reference

## Directory Layout
```
weather-service/
├── cmd/
│   └── server/
│       └── main.go                 # Application entry point
│
├── internal/                       # Private application code
│   ├── cache/
│   │   ├── cache.go               # In-memory cache with stale-on-error
│   │   └── cache_test.go          # Cache unit tests
│   │
│   ├── config/
│   │   └── config.go              # Environment-based configuration
│   │
│   ├── handlers/
│   │   ├── health.go              # Liveness probe endpoint
│   │   ├── metrics.go             # Prometheus metrics endpoint
│   │   └── weather.go             # Weather data endpoint
│   │
│   ├── metrics/
│   │   └── metrics.go             # Prometheus metric definitions
│   │
│   ├── middleware/
│   │   ├── correlationid.go       # Request correlation ID injection
│   │   ├── loadshed.go            # Load shedding middleware
│   │   ├── logging.go             # Structured logging middleware
│   │   ├── metrics.go             # Metrics collection middleware
│   │   ├── ratelimit.go           # Rate limiting middleware
│   │   └── ratelimit_test.go      # Rate limiter unit tests
│   │
│   └── weather/
│       ├── client.go              # OpenWeatherMap API client
│       └── client_test.go         # API client unit tests
│
├── deployments/
│   ├── kubernetes/
│   │   └── deployment.yaml        # K8s Deployment, Service, Secret
│   │
│   └── observability/
│       ├── alertmanager.yaml      # AlertManager configuration
│       ├── alerts.yaml            # Prometheus alert rules
│       └── prometheus.yaml        # Prometheus scrape config
│
├── bin/                            # Compiled binaries (gitignored)
│
├── Dockerfile                      # Multi-stage Docker build
├── .dockerignore                   # Docker build context exclusions
├── Makefile                        # Build automation
├── go.mod                          # Go module dependencies
├── go.sum                          # Dependency checksums
├── README.md                       # Main documentation
├── CLAUDE.md                       # Design decisions & architecture notes
├── DOCKER.md                       # Docker build & deployment guide
└── PROJECT_STRUCTURE.md            # This file
```

## Key Files by Concern

### Reliability Patterns
- `internal/weather/client.go` - Retry with exponential backoff
- `internal/cache/cache.go` - Stale-on-error fallback
- `internal/middleware/ratelimit.go` - Token bucket rate limiting
- `internal/middleware/loadshed.go` - Saturation protection

### Observability
- `internal/metrics/metrics.go` - 16 Prometheus metrics
- `internal/middleware/logging.go` - Structured logging with zerolog
- `internal/middleware/correlationid.go` - Distributed tracing support
- `deployments/observability/alerts.yaml` - SLO-based alerts

### Configuration
- `internal/config/config.go` - Environment variable loading
- `deployments/kubernetes/deployment.yaml` - K8s env vars & secrets

### Testing
- `internal/weather/client_test.go` - Tests retry logic (5xx vs 4xx)
- `internal/cache/cache_test.go` - Tests stale-on-error with age guard
- `internal/middleware/ratelimit_test.go` - Tests burst behavior

### Deployment
- `Dockerfile` - Multi-stage build (builder + distroless)
- `deployments/kubernetes/deployment.yaml` - K8s resources
- `deployments/observability/` - Prometheus & AlertManager configs

## Code Organization Principles

### `/cmd/server/main.go`
- Wiring layer: Initializes components and starts server
- Handles graceful shutdown (SIGTERM, 30s drain)
- Minimal business logic

### `/internal/*`
- Private packages (cannot be imported by external modules)
- Domain-driven organization (cache, weather, handlers, etc.)
- Each package has a single, clear responsibility

### `/internal/middleware/*`
- Composable HTTP middleware stack
- Each middleware is independent and testable
- Execution order matters (CorrelationID → Logger → Metrics → RateLimit → LoadShed)

### `/internal/metrics/*`
- Centralized metric definitions
- Uses promauto for automatic registration
- Exports metrics for use in handlers and clients

### `/deployments/*`
- Infrastructure as Code
- Separated by concern (kubernetes, observability)
- Ready for GitOps workflows

## Import Paths

All internal packages use the module path:
```go
import "weather-service/internal/config"
import "weather-service/internal/handlers"
import "weather-service/internal/metrics"
// etc.
```

External dependencies:
```go
import "github.com/go-chi/chi/v5"
import "github.com/rs/zerolog"
import "github.com/prometheus/client_golang/prometheus"
import "golang.org/x/time/rate"
import "github.com/google/uuid"
```

## Build Artifacts

### Generated Files (not in version control)
- `bin/weather-service` - Compiled binary
- `coverage.out` - Test coverage data
- `coverage.html` - Coverage report
- `go.sum` - Dependency checksums (auto-generated)

### Docker Artifacts
- Multi-stage build produces ~15MB final image
- Uses `gcr.io/distroless/static-debian12:nonroot` base
- Runs as UID 1000 (matches K8s security context)

## Middleware Execution Order

Request flow through middleware stack:
```
1. CorrelationID   → Injects/propagates X-Correlation-ID header
2. Logger          → Logs request start/completion
3. Metrics         → Tracks HTTP metrics (requests, duration, inflight)
4. RateLimit       → Returns 429 if rate exceeded
5. LoadShedding    → Returns 503 if service saturated
6. Handler         → Executes business logic
```

## Configuration Precedence

Environment variables with defaults:
```
1. WEATHER_API_KEY          (required, no default)
2. WEATHER_API_VERSION      (default: 2.5)
3. WEATHER_API_BASE_URL     (default: auto-detected based on API_VERSION)
                            - If VERSION=2.5: https://api.openweathermap.org/data/2.5
                            - If VERSION=3.0: https://api.openweathermap.org/data/3.0
                            - Can override manually if needed
4. CACHE_TTL_SECONDS        (default: 300)
5. RATE_LIMIT_RPS           (default: 50)
6. LOAD_SHED_THRESHOLD      (default: 100)
7. UPSTREAM_TIMEOUT_SECONDS (default: 5)
8. REQUEST_TIMEOUT_SECONDS  (default: 10)
9. LOG_LEVEL                (default: info)
10. PORT                    (default: 8080)
```

## Testing Strategy

### Unit Tests
- Test individual functions in isolation
- Mock external dependencies (HTTP servers)
- Focus on edge cases and error paths

### Coverage Target
- >70% on critical paths
- 100% on retry logic and cache stale-on-error
- Rate limiter, timeout handling, error classification

### No Integration Tests (Yet)
- Would require running actual OpenWeatherMap API calls
- Could add using test API key + VCR for recording/playback
- Future enhancement for pre-production validation

## Metrics Hierarchy

### SLI Metrics (Service Level Indicators)
- `http_request_duration_seconds` - Latency SLI
- `http_requests_total{status="5xx"}` - Error rate SLI

### Resource Metrics
- `http_inflight_requests` - Saturation indicator
- `cache_size_bytes` - Memory usage
- `go_goroutines` - Concurrency health

### Dependency Metrics
- `external_api_failures_total` - Upstream health
- `external_api_retries_total` - Retry frequency

### Reliability Metrics
- `cache_stale_served_total` - Degraded mode usage
- `load_shed_total` - Saturation protection activation
- `rate_limit_exceeded_total` - Client abuse detection

## Alert Routing

```
Severity: page
  ↓
PagerDuty (critical)
  - HighErrorRate (>1% for 2min)
  - UpstreamAPIFailures (>0.1/sec)
  - ServiceDown (>1min)

Severity: warning
  ↓
Slack #sre-alerts
  - HighLatency (P95 >500ms)
  - LoadSheddingActive (any events)
  - LowCacheHitRate (<50% for 10min)
```

## Design Patterns Used

1. **Middleware Pattern** - Composable request/response processing
2. **Repository Pattern** - Weather client abstracts external API
3. **Cache-Aside** - Application manages cache explicitly
4. **Circuit Breaker (Stale-on-Error variant)** - Graceful degradation
5. **Retry with Exponential Backoff** - Transient failure handling
6. **Load Shedding** - Saturation protection
7. **Graceful Shutdown** - Drain in-flight requests before exit

## Security Boundaries

### External Trust Boundary
- OpenWeatherMap API (untrusted)
- Client HTTP requests (untrusted)

### Internal Components
- All internal packages trusted
- No privilege separation within process

### Container Security
- Runs as non-root (UID 1000)
- Read-only root filesystem
- No capabilities
- Distroless image (no shell)

## Key Metrics Dashboard Layout

Suggested Grafana dashboard panels:

### Row 1: Golden Signals
- Request Rate (QPS)
- Error Rate (% 5xx)
- P50/P95/P99 Latency
- Saturation (Inflight Requests)

### Row 2: Dependency Health
- Upstream API Request Rate
- Upstream API Error Rate
- Upstream API Latency
- Retry Rate

### Row 3: Cache Performance
- Cache Hit Rate
- Cache Size
- Stale Served Count
- Cache Evictions

### Row 4: Resource Usage
- Memory Usage
- Goroutine Count
- Load Shedding Rate
- Rate Limit Violations

## Common Operations

### Scale Deployment
```bash
kubectl scale deployment weather-service --replicas=5
kubectl get pods -l app=weather-service -w
```

### View Logs
```bash
# All pods
kubectl logs -l app=weather-service --tail=100 -f

# Specific pod
kubectl logs weather-service-xxx-xxx
```

### Check Metrics
```bash
# Port forward to service
kubectl port-forward svc/weather-service 8080:80

# In another terminal, check metrics
curl http://localhost:8080/metrics
```

### Update Secret
```bash
kubectl delete secret weather-api-secret
./deployments/kubernetes/create-secret.sh new_api_key
kubectl rollout restart deployment/weather-service
```

### Check Resource Usage
```bash
# Pod CPU and memory
kubectl top pods -l app=weather-service

# Deployment resource configuration
kubectl describe deployment weather-service | grep -A 5 "Limits\|Requests"
```

### Switch API Versions
```bash
# Switch to API 2.5
kubectl apply -k deployments/kubernetes/overlays/api-2.5

# Switch to API 3.0
kubectl apply -k deployments/kubernetes/overlays/api-3.0

# Verify version
kubectl get deployment weather-service -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="WEATHER_API_VERSION")].value}'
```
