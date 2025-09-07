# Variables
BINARY_NAME=pfsense-container-controller
DOCKER_REGISTRY?=ghcr.io
DOCKER_IMAGE=$(DOCKER_REGISTRY)/kristijanl/pfsense-container-controller
VERSION?=latest
GO_VERSION=1.24

# Build the binary
build:
	go build -o $(BINARY_NAME) ./cmd/controller

# Build for Linux (useful for cross-compilation)
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(BINARY_NAME)-linux ./cmd/controller

# Clean build artifacts
clean:
	go clean
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_NAME)-linux

# Format code
fmt:
	go fmt ./...

# Run linting
lint:
	golangci-lint run

# Run tests
test:
	go test -v ./...

# Run tests with coverage
test-coverage:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Run the controller with example config
run:
	go run ./cmd/controller --config config/config.toml.example --log-level debug

# Download dependencies
deps:
	go mod download
	go mod tidy

# Build Docker image
docker-build:
	docker build -t $(DOCKER_IMAGE):$(VERSION) .

# Run Docker image
docker-run:
	docker run --rm -p 8080:8080 \
		-v /var/run/docker.sock:/var/run/docker.sock:ro \
		-v $(PWD)/config/config.toml.example:/etc/pfsense-controller/config.toml:ro \
		$(DOCKER_IMAGE):$(VERSION)

# Docker Compose up
compose-up:
	docker-compose -f deployments/docker-compose.yml up -d

# Docker Compose down
compose-down:
	docker-compose -f deployments/docker-compose.yml down

# Docker Compose logs
compose-logs:
	docker-compose -f deployments/docker-compose.yml logs -f pfsense-controller

# Install development tools
install-tools:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Generate API documentation (if using swag)
docs:
	@echo "API documentation not implemented yet"

# Release build (creates binaries for Linux platforms)
release:
	@mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o dist/$(BINARY_NAME)-linux-amd64 ./cmd/controller
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o dist/$(BINARY_NAME)-linux-arm64 ./cmd/controller

# Help
help:
	@echo "Available targets:"
	@echo "  build           - Build the binary"
	@echo "  build-linux     - Build for Linux"
	@echo "  clean           - Clean build artifacts"
	@echo "  fmt             - Format code"
	@echo "  lint            - Run linting"
	@echo "  test            - Run tests"
	@echo "  test-coverage   - Run tests with coverage"
	@echo "  run             - Run with example config"
	@echo "  deps            - Download and tidy dependencies"
	@echo "  docker-build    - Build Docker image"
	@echo "  docker-run      - Run Docker image"
	@echo "  compose-up      - Start with Docker Compose"
	@echo "  compose-down    - Stop Docker Compose"
	@echo "  compose-logs    - Show Docker Compose logs"
	@echo "  install-tools   - Install development tools"
	@echo "  release         - Build release binaries"
	@echo "  help            - Show this help"

.PHONY: build build-linux clean fmt lint test test-coverage run deps docker-build docker-run compose-up compose-down compose-logs install-tools docs release help