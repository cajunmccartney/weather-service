.PHONY: build test run clean lint

# Build the application
build:
	go build -o bin/weather-service cmd/server/main.go

# Run tests
test:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Run the application
run:
	go run cmd/server/main.go

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f coverage.out coverage.html

# Lint code
lint:
	golangci-lint run

# Run tests with coverage
test-coverage:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

# Docker build
docker-build:
	docker build -t weather-service:latest .

# Docker run
docker-run:
	docker run -p 8080:8080 \
		-e WEATHER_API_KEY=${WEATHER_API_KEY} \
		weather-service:latest
