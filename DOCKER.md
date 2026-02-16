# Docker Build & Deployment Guide

## Dockerfile Overview

The project uses a **multi-stage Dockerfile** optimized for size and security:

### Stage 1: Builder (golang:1.26-alpine)
- Compiles the Go binary with static linking (`CGO_ENABLED=0`)
- Strips debug symbols (`-ldflags="-w -s"`) for ~50% size reduction
- Downloads dependencies separately for better layer caching

### Stage 2: Runtime (distroless/static-debian12)
- **Minimal attack surface**: No shell, package manager, or unnecessary utilities
- **Distroless image**: Only contains the Go binary and runtime dependencies
- **Security**: Runs as non-root user (UID 1000) matching K8s security context
- **Size**: Final image ~15-20MB (vs ~1GB for full Go base image)

## Building the Image

```bash
# Build with default tag
docker build -t weather-service:latest .

# Build with specific version tag
docker build -t weather-service:latest .

# Build for specific platform (e.g., for Minikube on ARM Mac)
docker buildx build --platform linux/amd64 -t weather-service:latest .
```

## Running with Docker

### Basic Usage

```bash
# Run with API 2.5 (default, free, no payment method)
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

# Run with additional configuration
docker run -d -p 8080:8080 \
  --name weather-service \
  -e WEATHER_API_KEY=your_key_here \
  -e WEATHER_API_VERSION=2.5 \
  -e LOG_LEVEL=debug \
  -e CACHE_TTL_SECONDS=600 \
  -e RATE_LIMIT_RPS=100 \
  weather-service:latest
```

### Docker Compose

Create `docker-compose.yaml`:

```yaml
version: '3.8'

services:
  weather-service:
    build: .
    image: weather-service:latest
    ports:
      - "8080:8080"
    environment:
      - WEATHER_API_KEY=${WEATHER_API_KEY}
      - WEATHER_API_VERSION=2.5
      - LOG_LEVEL=info
      - CACHE_TTL_SECONDS=300
      - RATE_LIMIT_RPS=50
      - LOAD_SHED_THRESHOLD=100
      - UPSTREAM_TIMEOUT_SECONDS=5
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "--spider", "--quiet", "http://localhost:8080/health"]
      interval: 30s
      timeout: 5s
      retries: 3
```

Then run:

```bash
export WEATHER_API_KEY=your_key_here
docker-compose up -d

# View logs
docker-compose logs -f

# Stop
docker-compose down
```

## Operations

### View Logs

```bash
# Follow logs in real-time
docker logs -f weather-service

# View last 100 lines
docker logs --tail 100 weather-service

# View logs since specific time
docker logs --since 10m weather-service

# View logs with timestamps
docker logs -t weather-service
```

### Restart and Update

```bash
# Restart container
docker restart weather-service

# Stop container
docker stop weather-service

# Start stopped container
docker start weather-service

# Remove container
docker rm weather-service

# Stop and remove
docker stop weather-service && docker rm weather-service
```

### Update Configuration

```bash
# Stop existing container
docker stop weather-service && docker rm weather-service

# Run with new configuration
docker run -d -p 8080:8080 \
  --name weather-service \
  -e WEATHER_API_KEY=new_key \
  -e WEATHER_API_VERSION=3.0 \
  weather-service:latest
```

### Monitor Resource Usage

```bash
# Real-time stats
docker stats weather-service

# One-time stats
docker stats --no-stream weather-service

# View container processes
docker top weather-service
```

### Health Checks

```bash
# Check if container is healthy
docker inspect --format='{{.State.Health.Status}}' weather-service

# View health check logs
docker inspect --format='{{range .State.Health.Log}}{{.Output}}{{end}}' weather-service

# Manual health check
curl http://localhost:8080/health
```

### Testing

```bash
# Test endpoints
curl http://localhost:8080/health
curl http://localhost:8080/weather/London
curl http://localhost:8080/weather/Tokyo
curl http://localhost:8080/metrics

# Test with coordinates (API 3.0 only)
curl http://localhost:8080/weather/51.5074,-0.1278

# Check metrics
curl http://localhost:8080/metrics | grep external_api_requests_total
curl http://localhost:8080/metrics | grep cache_hits_total
curl http://localhost:8080/metrics | grep http_request_duration_seconds
```

### Debugging

```bash
# Inspect container
docker inspect weather-service

# Check container logs for errors
docker logs weather-service 2>&1 | grep -i error

# Execute command in running container (Note: distroless has no shell)
# You'll need to use the debug variant for this
docker exec -it weather-service /bin/sh  # Won't work with distroless

# Check environment variables
docker exec weather-service env | grep WEATHER
```

## Image Size Optimization

### Current Image Size
- **Builder stage**: ~600MB (not in final image)
- **Final image**: ~15-20MB

### Why So Small?
1. **Distroless base**: No OS, shell, or package manager
2. **Static binary**: No dynamic linking dependencies
3. **Stripped symbols**: Debug info removed with `-ldflags="-w -s"`
4. **Multi-stage build**: Build dependencies not included in final image

### Size Comparison
```
golang:1.26-alpine (full)    ~300MB
golang:1.26-alpine (binary)  ~15MB  (with Alpine)
distroless/static             ~2MB   (base)
weather-service:latest        ~15MB  (final)
```

## Security Features

### 1. Distroless Image
- No shell → Cannot execute arbitrary commands
- No package manager → Cannot install malware
- Minimal CVE surface area
- Compliant with SLSA Level 3 supply chain security

### 2. Non-Root User
```dockerfile
USER 1000:1000
```
- Prevents privilege escalation
- Aligns with least-privilege principle
- Matches K8s security context

### 3. Static Binary
```bash
CGO_ENABLED=0
```
- No C library dependencies
- No libc vulnerabilities
- Portable across Linux distributions

## Troubleshooting

### Image Won't Build
```bash
# Clear Docker cache and rebuild
docker builder prune -a
docker build --no-cache -t weather-service:latest .
```

### Binary Won't Run in Container
Check that `CGO_ENABLED=0` is set:
```bash
# Inspect binary
docker run --rm weather-service:latest /app/weather-service --version
# Should not require glibc
```

### Application Crashes on Startup
```bash
# Check logs
docker logs weather-service

# Verify environment variables
docker exec weather-service env | grep WEATHER

# Check if API key is set
docker inspect weather-service | grep WEATHER_API_KEY
```

### Metrics Not Appearing
The Go runtime collectors (`go_*`, `process_*`) are automatically registered by Prometheus. They should appear at `/metrics` without any manual registration.

```bash
# Verify metrics endpoint
curl http://localhost:8080/metrics | grep go_goroutines
curl http://localhost:8080/metrics | grep process_cpu_seconds_total
```

### Permission Denied Errors
The distroless image runs as UID 1000 by default. Ensure this matches your environment's expectations.

### Port Already in Use
```bash
# Find process using port 8080
lsof -i :8080
# Or on Linux:
netstat -tulpn | grep 8080

# Stop the conflicting process or use a different port
docker run -d -p 8081:8080 \
  --name weather-service \
  -e WEATHER_API_KEY=your_key_here \
  weather-service:latest
```

## Advanced Usage

### Building for Multiple Platforms
```bash
# Build for AMD64 and ARM64
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t weather-service:latest \
  --push \
  .
```

### Scanning for Vulnerabilities
```bash
# Using Trivy
trivy image weather-service:latest

# Using Docker Scout
docker scout cves weather-service:latest

# Using Grype
grype weather-service:latest
```

### Inspecting the Image
```bash
# View layers
docker history weather-service:latest

# Check image size
docker images weather-service:latest

# Inspect metadata
docker inspect weather-service:latest

# Export image
docker save weather-service:latest > weather-service.tar

# Load image
docker load < weather-service.tar
```

### Using Debug Container (distroless:debug)
For troubleshooting, temporarily switch to the debug variant:
```dockerfile
# In Dockerfile, change the runtime image to:
FROM gcr.io/distroless/static-debian12:debug

# This adds busybox shell for debugging
```

Then you can exec into the container:
```bash
docker exec -it weather-service /busybox/sh
```

**Note:** Never use the debug image in production.

## Production Recommendations

### 1. Tag Images with Git SHA
```bash
# Tag with commit hash
docker build -t weather-service:$(git rev-parse --short HEAD) .

# Tag with semantic version
docker build -t weather-service:v1.0.0 .
```

### 2. Use Image Scanning in CI/CD
```yaml
# GitHub Actions example
- name: Build image
  run: docker build -t ${{ env.IMAGE_NAME }}:${{ github.sha }} .

- name: Scan image
  run: trivy image --severity HIGH,CRITICAL ${{ env.IMAGE_NAME }}:${{ github.sha }}
```

### 3. Sign Images with Cosign
```bash
# Generate keys
cosign generate-key-pair

# Sign image
cosign sign --key cosign.key weather-service:latest

# Verify signature
cosign verify --key cosign.pub weather-service:latest
```

### 4. Use Private Container Registry
```bash
# Tag for registry
docker tag weather-service:latest registry.example.com/weather-service:latest

# Push to registry
docker push registry.example.com/weather-service:latest

# Pull from registry
docker pull registry.example.com/weather-service:latest
```

## Performance Testing

### Load Testing with Apache Bench
```bash
# Test request rate
ab -n 1000 -c 20 http://localhost:8080/weather/London

# Test rate limiting
ab -n 200 -c 10 http://localhost:8080/weather/London

# Test load shedding
ab -n 10000 -c 200 http://localhost:8080/weather/London
```

### Load Testing with Hey
```bash
# Install hey
go install github.com/rakyll/hey@latest

# Run load test
hey -n 1000 -c 50 http://localhost:8080/weather/London

# With custom headers
hey -n 1000 -c 50 \
  -H "X-Correlation-ID: test-123" \
  http://localhost:8080/weather/London
```

## References

- [Distroless Images](https://github.com/GoogleContainerTools/distroless)
- [Go Multi-Stage Builds](https://docs.docker.com/language/golang/build-images/)
- [Docker Best Practices](https://docs.docker.com/develop/dev-best-practices/)
- [SLSA Framework](https://slsa.dev/)
