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
docker build -t weather-service:v1.0.0 .

# Build for specific platform (e.g., for Minikube on ARM Mac)
docker buildx build --platform linux/amd64 -t weather-service:latest .
```

## Deployment Options

### Option 1: Kubernetes/Minikube (Recommended)

This is the **recommended deployment method** as it provides the most production-like environment with proper health checks, resource limits, and observability.

#### Step 1: Build and Load Image
```bash
# Build the Docker image
docker build -t weather-service:latest .

# Load image into Minikube's Docker daemon
minikube image load weather-service:latest

# Verify image is loaded
minikube image ls | grep weather-service
```

#### Step 2: Create Secret
```bash
# Create secret with your OpenWeatherMap API key
./deployments/kubernetes/create-secret.sh your_openweathermap_api_key
```

Alternative (manual method):
```bash
kubectl create secret generic weather-api-secret \
  --from-literal=api-key=your_openweathermap_api_key
```

#### Step 3: Deploy to Kubernetes
```bash
# Deploy the application
kubectl apply -f deployments/kubernetes/deployment.yaml

# Verify deployment
kubectl get pods -l app=weather-service
kubectl get svc weather-service
```

#### Step 4: Access the Service
```bash
# Port forward to access locally
kubectl port-forward svc/weather-service 8080:80

# Test the endpoints
curl http://localhost:8080/health
curl http://localhost:8080/weather/London
curl http://localhost:8080/metrics
```

#### Step 5: Monitor and Debug
```bash
# View logs
kubectl logs -l app=weather-service --tail=100 -f

# Check pod status
kubectl describe pod -l app=weather-service

# Check resource usage
kubectl top pods -l app=weather-service

# Access pod shell (for debugging - note: distroless has no shell)
kubectl exec -it <pod-name> -- /bin/sh  # Won't work with distroless
```

#### Clean Up
```bash
# Delete deployment
kubectl delete -f deployments/kubernetes/deployment.yaml

# Delete secret
kubectl delete secret weather-api-secret
```

### Option 2: Docker (Standalone)

For quick local testing without Kubernetes orchestration.

#### Run with Docker
```bash
# Run with environment variables
docker run -p 8080:8080 \
  -e WEATHER_API_KEY=your_api_key_here \
  -e LOG_LEVEL=debug \
  weather-service:latest

# Run in background
docker run -d -p 8080:8080 \
  --name weather-service \
  -e WEATHER_API_KEY=your_api_key_here \
  weather-service:latest

# View logs
docker logs -f weather-service

# Stop and remove container
docker stop weather-service && docker rm weather-service
```

#### Run with Docker Compose
Create `docker-compose.yml`:
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

### Option 3: Native Go (Development)

For active development and testing without containerization overhead.

#### Prerequisites
- Go 1.26+ installed
- OpenWeatherMap API key

#### Build and Run
```bash
# Build binary
go build -o bin/weather-service cmd/server/main.go

# Run with environment variables
export WEATHER_API_KEY=your_key_here
export LOG_LEVEL=debug
./bin/weather-service

# Or use make
make build
export WEATHER_API_KEY=your_key_here
make run
```

#### Run Tests
```bash
# Run all tests
go test -v ./...

# Run with coverage
go test -v -race -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Or use make
make test
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
- Matches K8s `securityContext.runAsUser: 1000`
- Prevents privilege escalation
- Aligns with least-privilege principle

### 3. Static Binary
```bash
CGO_ENABLED=0
```
- No C library dependencies
- No libc vulnerabilities
- Portable across Linux distributions

### 4. Read-Only Filesystem (K8s)
K8s manifest sets:
```yaml
securityContext:
  readOnlyRootFilesystem: true
```
- Prevents runtime modifications
- No persistent malware installation
- Immutable infrastructure pattern

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

### Minikube Can't Find Image
```bash
# Make sure you're using Minikube's Docker daemon
eval $(minikube docker-env)
docker build -t weather-service:latest .

# Or load after building locally
minikube image load weather-service:latest

# Verify image exists
minikube image ls | grep weather-service
```

### Application Crashes on Startup
```bash
# Check pod logs
kubectl logs -l app=weather-service --tail=50

# Check if secret exists
kubectl get secret weather-api-secret

# Verify secret has correct key name
kubectl describe secret weather-api-secret

# Check environment variables in pod
kubectl exec <pod-name> -- env | grep WEATHER
```

### Metrics Not Appearing
The Go runtime collectors (`go_*`, `process_*`) are automatically registered by Prometheus. They should appear at `/metrics` without any manual registration.

```bash
# Verify metrics endpoint
curl http://localhost:8080/metrics | grep go_goroutines
curl http://localhost:8080/metrics | grep process_cpu_seconds_total
```

### Permission Denied Errors
The distroless image runs as UID 1000 by default. Ensure this matches your K8s security context.

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
kubectl exec -it <pod-name> -- /busybox/sh
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

# Update K8s deployment to use private registry
# Add imagePullSecrets to deployment.yaml
```

### 5. Implement Automated Deployments
```bash
# Using ArgoCD
kubectl apply -f argocd/application.yaml

# Using Flux
flux create source git weather-service \
  --url=https://github.com/org/weather-service

flux create kustomization weather-service \
  --source=weather-service \
  --path=./deployments/kubernetes
```

## Monitoring in Kubernetes

### Deploy Prometheus Stack
```bash
# Add Prometheus Helm repo
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update

# Install Prometheus + Grafana
helm install prometheus prometheus-community/kube-prometheus-stack

# Port forward to access Prometheus
kubectl port-forward svc/prometheus-kube-prometheus-prometheus 9090:9090

# Port forward to access Grafana
kubectl port-forward svc/prometheus-grafana 3000:80
```

### Deploy AlertManager Configuration
```bash
# Create ConfigMap from alerts.yml
kubectl create configmap weather-service-alerts \
  --from-file=deployments/observability/alerts.yml

# Configure Prometheus to load the rules
# (This is typically done through the Prometheus Operator)
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
- [Kubernetes Best Practices](https://kubernetes.io/docs/concepts/configuration/overview/)
- [SLSA Framework](https://slsa.dev/)
- [Prometheus Operator](https://prometheus-operator.dev/)
