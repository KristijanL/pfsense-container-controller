# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o pfsense-container-controller ./cmd/controller

# Final stage
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates curl tzdata

# Create non-root user
RUN addgroup -g 1000 -S pfsense && \
    adduser -u 1000 -S pfsense -G pfsense

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/pfsense-container-controller .

# Copy example configuration
COPY --from=builder /app/config/config.toml.example /etc/pfsense-controller/config.toml.example

# Create config directory and set permissions
RUN mkdir -p /etc/pfsense-controller && \
    chown -R pfsense:pfsense /app /etc/pfsense-controller

# Switch to non-root user
USER pfsense

# Expose health check port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

# Set default environment variables
ENV PFSENSE_LOG_LEVEL=info \
    PFSENSE_POLL_INTERVAL=30s \
    PFSENSE_HEALTH_PORT=8080

# Run the controller
ENTRYPOINT ["./pfsense-container-controller"]
CMD ["--config", "/etc/pfsense-controller/config.toml"]