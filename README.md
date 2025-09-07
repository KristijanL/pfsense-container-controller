# pfSense Container Controller

[![License: MPL 2.0](https://img.shields.io/badge/License-MPL%202.0-brightgreen.svg)](https://opensource.org/licenses/MPL-2.0)
[![Go Version](https://img.shields.io/badge/Go-1.24+-blue.svg)](https://golang.org/dl/)
[![Docker](https://img.shields.io/badge/Docker-Compatible-blue.svg)](https://www.docker.com/)

A container controller that monitors Docker/Podman/CRI-O containers and automatically manages pfSense HAProxy configurations based on container labels.

## Features

- **Multi-Runtime Support**: Works with Docker, Podman, and CRI-O
- **Multiple pfSense Endpoints**: Manage multiple pfSense instances
- **Dual Label Support**: Native pfSense labels + Traefik compatibility mode
- **Automatic Discovery**: Continuously monitors container events
- **Retry Logic**: Configurable retry mechanisms for reliability
- **Health Checks**: Built-in health and readiness endpoints
- **Metrics**: Prometheus-compatible metrics

## Quick Start

### 1. Configuration

Create a configuration file or use environment variables:

```toml
# /etc/pfsense-controller/config.toml
[global]
poll_interval = "30s"
retry_attempts = 3
retry_delay = "5s"
log_level = "info"
health_port = 8080
traefik_compat_mode = false  # Enable to reuse existing Traefik labels

[[endpoints]]
name = "production"
url = "https://your-pfsense.example.com/api/v2"
api_key = "your-api-key-here"
insecure_tls = false
request_timeout = "30s"
```

### 2. Container Labels

The controller supports two label modes:

#### Native Mode (Default)
```yaml
version: '3.8'
services:
  my-service:
    image: nginx:latest
    labels:
      # Enable the controller
      pfsense-controller.enable: "true"
      
      # Specify pfSense endpoint (optional)
      pfsense-controller.endpoint: "production"
      
      # Backend configuration
      pfsense-controller.backend.name: "my-service-backend"
      pfsense-controller.backend.port: "80"
      pfsense-controller.backend.check_type: "http"  # none, basic, http
      pfsense-controller.backend.health_check_path: "/health"  # required for http
      pfsense-controller.backend.health_check_method: "OPTIONS"  # default OPTIONS
      pfsense-controller.backend.server_name: "my-service"
      
      # Frontend configuration
      pfsense-controller.frontend.name: "my-service-frontend"
      pfsense-controller.frontend.rule: "Host(`my-service.example.com`)"
      pfsense-controller.frontend.acl_name: "my-service-acl"
```

#### Traefik Compatibility Mode
Set `traefik_compat_mode = true` to reuse existing Traefik labels for backend configuration:

```yaml
version: '3.8'
services:
  my-service:
    image: nginx:latest
    labels:
      # Existing Traefik labels (used for backend)
      traefik.enable: "true"
      traefik.http.services.my-service.loadbalancer.server.port: "80"
      
      # pfSense controller labels for frontend (still required)
      pfsense-controller.frontend.name: "my-service-frontend"
      pfsense-controller.frontend.rule: "Host(`my-service.example.com`)"
      pfsense-controller.frontend.acl_name: "my-service-acl"
      
      # Optional: Override health check type (default: basic for Traefik mode)
      pfsense-controller.backend.check_type: "http"
      pfsense-controller.backend.health_check_path: "/health"
```

### 3. Running the Controller

```bash
# Using configuration file
./pfsense-container-controller --config /etc/pfsense-controller/config.toml

# Using environment variables
export PFSENSE_URL="https://your-pfsense.example.com/api/v2"
export PFSENSE_API_KEY="your-api-key-here"
./pfsense-container-controller
```

## Label Schema

### Core Labels

| Label | Required | Description |
|-------|----------|-------------|
| `pfsense-controller.enable` | ✅ | Set to `"true"` to enable the controller for this container |
| `pfsense-controller.endpoint` | ❌ | pfSense endpoint name (defaults to first configured endpoint) |

### Backend Labels

| Label | Required | Description | Default |
|-------|----------|-------------|---------|
| `pfsense-controller.backend.name` | ❌ | HAProxy backend name | `{container-name}-backend` |
| `pfsense-controller.backend.port` | ✅ | Container port to proxy to | - |
| `pfsense-controller.backend.check_type` | ❌ | Health check type | `basic` |
| `pfsense-controller.backend.health_check_path` | ❌* | Health check endpoint | - |
| `pfsense-controller.backend.health_check_method` | ❌ | HTTP method for health check | `OPTIONS` |
| `pfsense-controller.backend.server_name` | ❌ | Server name in backend | `{container-name}` |

*Required when `check_type` is `http`

### Frontend Labels

| Label | Required | Description | Default |
|-------|----------|-------------|---------|
| `pfsense-controller.frontend.name` | ❌ | HAProxy frontend name | Auto-generated |
| `pfsense-controller.frontend.rule` | ✅ | Routing rule (Traefik syntax) | - |
| `pfsense-controller.frontend.acl_name` | ❌ | ACL name | Auto-generated |

## Supported Rule Formats

The controller supports Traefik-style routing rules:

- **Host Rules**: `Host(\`example.com\`)`
- **Path Rules**: `Path(\`/api\`)`
- **Path Prefix Rules**: `PathPrefix(\`/api\`)`

## Health Check Types

The controller supports three health check types:

- **`none`**: No health checks performed
- **`basic`**: TCP connection check (default)
- **`http`**: HTTP-based health check with configurable path and method

## Frontend Reuse

When multiple containers specify the same frontend name, the controller will:
1. Create the frontend for the first container
2. Add additional ACLs and routing rules to the existing frontend for subsequent containers
3. This allows multiple services to share the same frontend with different routing rules

## Configuration

### TOML Configuration File

```toml
[global]
poll_interval = "30s"        # How often to scan containers
retry_attempts = 3           # API retry attempts
retry_delay = "5s"          # Delay between retries
log_level = "info"          # Log level
health_port = 8080          # Health server port

[[endpoints]]
name = "production"
url = "https://pfsense.example.com/api/v2"
api_key = "your-api-key"
insecure_tls = false
request_timeout = "30s"
```

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `PFSENSE_URL` | pfSense API URL | - |
| `PFSENSE_API_KEY` | API key | - |
| `PFSENSE_INSECURE_TLS` | Skip TLS verification | `false` |
| `PFSENSE_POLL_INTERVAL` | Poll interval | `30s` |
| `PFSENSE_LOG_LEVEL` | Log level | `info` |
| `PFSENSE_HEALTH_PORT` | Health server port | `8080` |
| `PFSENSE_TRAEFIK_COMPAT_MODE` | Enable Traefik compatibility | `false` |

## API Endpoints

The controller provides several HTTP endpoints for monitoring:

- `GET /health` - Health check endpoint
- `GET /ready` - Readiness check endpoint  
- `GET /metrics` - Prometheus metrics

## Docker Example

Complete Docker Compose example:

```yaml
version: '3.8'
services:
  pfsense-controller:
    image: ghcr.io/kristijanl/pfsense-container-controller:latest
    restart: unless-stopped
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - ./config.toml:/etc/pfsense-controller/config.toml:ro
    environment:
      - PFSENSE_LOG_LEVEL=info
    ports:
      - "8080:8080"  # Health/metrics port

  web-service:
    image: nginx:alpine
    labels:
      pfsense-controller.enable: "true"
      pfsense-controller.backend.port: "80"
      pfsense-controller.frontend.rule: "Host(\`web.example.com\`)"

  # Example with Traefik compatibility (set PFSENSE_TRAEFIK_COMPAT_MODE=true)
  traefik-service:
    image: nginx:alpine
    labels:
      # Existing Traefik labels
      traefik.enable: "true"
      traefik.http.services.traefik-service.loadbalancer.server.port: "80"
      
      # pfSense frontend labels (still required)
      pfsense-controller.frontend.rule: "Host(\`traefik.example.com\`)"
```

## Building from Source

```bash
# Clone the repository
git clone https://github.com/KristijanL/pfsense-container-controller.git
cd pfsense-container-controller

# Download dependencies
go mod download

# Build the binary
go build -o pfsense-container-controller ./cmd/controller

# Run with example configuration
./pfsense-container-controller --config config/config.toml.example

# Or use make
make build
make run
```

### Development Setup

```bash
# Install development tools
make install-tools

# Format code
make fmt

# Run linting
make lint

# Run tests
make test

# Build for multiple platforms
make release
```

## pfSense API Key Setup

1. Log into your pfSense web interface
2. Navigate to **System → REST API → Keys**
3. Click **Add** to create a new API key
4. Set the required privileges (HAProxy management)
5. Save and use the generated key in your configuration

## Monitoring

The controller provides metrics in Prometheus format at `/metrics`:

```
# HELP pfsense_controller_syncs_total Total number of sync operations
# TYPE pfsense_controller_syncs_total counter
pfsense_controller_syncs_total 42

# HELP pfsense_controller_errors_total Total number of errors
# TYPE pfsense_controller_errors_total counter
pfsense_controller_errors_total 0

# HELP pfsense_haproxy_backends Number of HAProxy backends
# TYPE pfsense_haproxy_backends gauge
pfsense_haproxy_backends{endpoint="production"} 5
```

## Troubleshooting

### Container Not Being Managed

1. Check if the container has `pfsense-controller.enable: "true"` label
2. Verify the container is running
3. Check controller logs for any parsing errors

### API Connection Issues

1. Verify pfSense API key has correct permissions
2. Check network connectivity to pfSense instance
3. Ensure pfSense REST API is enabled

### Configuration Issues

1. Validate TOML syntax in configuration file
2. Check environment variable names and values
3. Verify endpoint URLs are accessible

## Contributing

We welcome contributions! Please see our contribution guidelines:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes
4. Add tests if applicable
5. Commit your changes (`git commit -m 'Add amazing feature'`)
6. Push to the branch (`git push origin feature/amazing-feature`)
7. Open a Pull Request

### Code Style

- Follow Go conventions and best practices
- Use `gofmt` to format your code
- Run `golangci-lint` before submitting
- Write clear commit messages
- Add documentation for new features

### Reporting Issues

Please use GitHub issues to report bugs or request features. Include:
- OS and container runtime details
- pfSense version and API configuration
- Container labels and configuration
- Relevant logs and error messages

## License

This project is licensed under the Mozilla Public License Version 2.0 - see the [LICENSE](LICENSE) file for details.

[![License: MPL 2.0](https://img.shields.io/badge/License-MPL%202.0-brightgreen.svg)](https://opensource.org/licenses/MPL-2.0)

## Acknowledgments

- Inspired by Traefik's label-based configuration
- Built for the pfSense community
- Uses the excellent [pfSense REST API](https://github.com/jaredhendrickson13/pfsense-api)