# Multi-stage Dockerfile for weather-service
# Stage 1: Build the Go binary
FROM golang:1.26-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
# CGO_ENABLED=0 for static binary
# -ldflags="-w -s" strips debug info for smaller binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o weather-service \
    cmd/server/main.go

# Stage 2: Create minimal runtime image
FROM gcr.io/distroless/static-debian12:nonroot

# Copy CA certificates from builder
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy timezone data (needed for time.LoadLocation)
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/weather-service /app/weather-service

# Expose port
EXPOSE 8080

# distroless/static:nonroot runs as UID 65532 by default
# We need to run as UID 1000 to match K8s security context
USER 1000:1000

# Set entrypoint
ENTRYPOINT ["/app/weather-service"]
