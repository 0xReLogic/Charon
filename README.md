# Charon

A lightweight, high-performance service mesh sidecar proxy built with Go, designed for transparently securing and observing microservice traffic.

## Overview

Charon acts as a "bodyguard" for your microservices, intercepting all incoming and outgoing traffic, verifying identities, logging activities, and ensuring messages reach their correct destination, even if the destination moves.

## Features

### Implemented
- **Transparent TCP Proxy**: Forward TCP traffic without application awareness
- **Intelligent HTTP Proxy**: Parse and understand HTTP traffic with metrics
- **Service Discovery**: File-based registry with dynamic routing and cache/watcher
- **Advanced Routing**: Host/path-based routing with multi-upstream support
- **Health Checks**: Active TCP probes and passive health monitoring
- **Circuit Breaking**: Per-upstream circuit breaker with configurable thresholds and timeouts
- **Rate Limiting**: Token bucket algorithm with configurable RPS and burst limits
- **Retry Logic**: Exponential backoff for idempotent requests
- **Structured Logging**: Zap logger with trace context and structured fields
- **Distributed Tracing**: OpenTelemetry integration with Jaeger exporter
- **Secure Communication**: Automatic mTLS with certificate generation and management
- **Prometheus Metrics**: Request metrics, latencies, health status, CB transitions, and rate limiting

## Getting Started

### Prerequisites

- Go 1.21 or higher

### Installation

```bash
# Clone the repository
git clone https://github.com/0xReLogic/Charon.git
cd Charon

# Build the project
go mod tidy
go build -o charon.exe ./cmd/charon
```

### Configuration

Create a `config.yaml` file:

```yaml
listen_port: "8080"
target_service_name: "my-backend"
registry_file: "registry.yaml"

routes:
  - host: "api.example.com"
    path_prefix: "/v1"
    service: "api-service"
  - path_prefix: "/users"
    service: "user-service"

# Circuit breaker settings
circuit_breaker:
  failure_threshold: 5
  open_duration: "30s"

# Rate limiting settings  
rate_limit:
  requests_per_second: 100
  burst_size: 200
  routes: []  # empty = all routes

logging:
  level: "info"
  format: "json"
  environment: "production"

tracing:
  enabled: false
  jaeger_endpoint: "http://localhost:14268/api/traces"
  service_name: "charon-proxy"

# TLS/mTLS Configuration
tls:
  enabled: false
  cert_dir: "./certs"
  server_port: "8443"
  upstream_tls: false
```

### Running

```bash
# Start the proxy
./charon --config config.yaml          # macOS/Linux
# or on Windows
./charon.exe --config config.yaml

# In another terminal, start the echo server for testing
go run ./test/cmd/echo_server --port 9091
```

## Testing

You can test the proxy using telnet or netcat:

```bash
# Connect to the proxy
telnet localhost 8080

# Type any message and it should be echoed back
```

Alternatively, use the provided test clients:

```bash
# Smoke test (expects echo back)
go run ./test/cmd/smoke_client --addr localhost:8080 --msg "hello-through-proxy\n"

# Interactive client (type and see echo)
go run ./test/cmd/interactive_client --addr localhost:8080
```

### Phase 2: HTTP Reverse Proxy Testing

Start a simple HTTP backend, run Charon, then curl via the proxy:

```bash
# Terminal A: HTTP backend on :9091
go run ./test/cmd/http_backend --addr :9091

# Terminal B: Charon HTTP reverse proxy on :8080
./charon.exe --config config.yaml    # or ./charon on Linux/macOS

# Terminal C: Send requests through proxy
curl -v http://localhost:8080/
curl -v -X POST http://localhost:8080/hello -d 'hi'
```

You should see logs like:

```
http request method=GET path=/ -> status=200 bytes=... latency=...
```

### Phase 3: Service Discovery (Dynamic Routing)

Use a file-based registry to dynamically resolve upstream addresses per request (no restart needed).

1) Create `registry.yaml`:

```yaml
services:
  http-backend: localhost:9091
```

2) Start backend and Charon:

```bash
# Terminal A: backend on :9091
go run ./test/cmd/http_backend --addr :9091

# Terminal B: start Charon
./charon.exe --config config.yaml

# Terminal C: call via proxy
curl -v http://localhost:8080/hello
```

3) Change routing dynamically:

```yaml
# edit registry.yaml
services:
  http-backend: localhost:9092
```

Restart backend on :9092:

```bash
go run ./test/cmd/http_backend --addr :9092
```

Call again. The proxy will route to the new address without restart.

### Observability: Prometheus Metrics

Charon exposes Prometheus metrics at `/metrics` on the same listen port.

Example:

```bash
curl -s http://localhost:8080/metrics | grep charon_
```

Available metrics include:

- `charon_http_requests_total{method,status,upstream}`
- `charon_http_request_latency_seconds_bucket{method,upstream,...}` (+ sum/count)
- `charon_http_retries_total{method}`
- `charon_http_rate_limited_total{route}` (counter)
- `charon_upstream_health{service,upstream}` (gauge 1=UP, 0=DOWN)
- `charon_circuit_breaker_transitions_total{upstream,to_state}` (counter)

You can configure Prometheus to scrape `http://<charon-host>:8080/metrics`.

### Circuit Breaker & Health Checks

Charon performs active health checks (TCP probe every 5s) and per-upstream circuit breaking.

- Circuit breaker: configurable failure threshold and open duration (defaults: 3 failures, 20s).
- Rate limiting: token bucket algorithm with configurable RPS and burst size.
- Metrics:
  - `charon_upstream_health{service,upstream}`: current health.
  - `charon_circuit_breaker_transitions_total{upstream,to_state}`: transitions (open/half_open/closed).
  - `charon_http_rate_limited_total{route}`: rate limited requests per route.

Test the circuit breaker locally:

```yaml
# registry.yaml (temporary for deterministic testing)
services:
  http-backend: localhost:9091
```

```bash
# Terminal A: backend with failing endpoint
go run ./test/cmd/http_backend --addr :9091   # /fail returns 500

# Terminal B: start Charon
./charon.exe --config config.yaml

# Terminal C (PowerShell): trip breaker with 500s
1..10 | % { curl.exe -s http://localhost:8080/fail > $null }

# Check transitions metric
curl -s http://localhost:8080/metrics | findstr charon_circuit_breaker_transitions_total

# Wait ~21s (half-open), then send a success to close
Start-Sleep -Seconds 21; curl.exe -s http://localhost:8080/hello > $null
curl -s http://localhost:8080/metrics | findstr charon_circuit_breaker_transitions_total
```

### Advanced Routing (Host/Path)

Charon mendukung routing berbasis host/path melalui `routes` di `config.yaml`.
Aturan dievaluasi dari atas ke bawah, first match wins. Jika tidak ada yang match,
akan fallback ke `target_service_name` (jika ada) atau `target_service_addr`.

Contoh konfigurasi:

```yaml
listen_port: "8080"

# Default service (fallback)
target_service_name: "http-backend"
registry_file: "registry.yaml"

# Routing rules (opsional)
routes:
  - host: "api.local"
    path_prefix: "/hello"
    service: "http-backend"     # nama service di registry.yaml
  - path_prefix: "/admin"
    service: "admin-backend"
```

Contoh `registry.yaml`:

```yaml
services:
  http-backend: localhost:9091
  admin-backend: localhost:9092
```

Uji cepat:

```bash
curl -v http://localhost:8080/hello              # -> http-backend
curl -v http://localhost:8080/admin               # -> admin-backend
# atau dengan Host header spesifik:
curl -v -H "Host: api.local" http://localhost:8080/hello
```

## Project Structure

```
charon/
├── cmd/
│   └── charon/          # Main application entry point
├── internal/
│   ├── config/          # Configuration handling
│   ├── proxy/           # Proxy implementation
│   │   ├── tcp.go       # Phase 1: TCP transparent proxy
│   │   └── http.go      # Phase 2: HTTP reverse proxy with basic metrics
│   ├── registry/        # Phase 3: file-based service discovery
│   └── ...
├── test/                # Test utilities and mock servers
│   ├── cmd/             # Standalone test binaries (no conflict with library)
│   │   ├── echo_server/
│   │   ├── smoke_client/
│   │   └── http_backend/
│   │   └── interactive_client/
│   ├── echo_server.go   # Library: RunEchoServer
│   ├── smoke_client.go  # Library: RunSmokeClient
│   └── test_proxy.go    # Library: RunInteractiveProxyClient
├── config.yaml          # Default configuration (Phase 3 by default)
├── registry.yaml        # Sample service registry (Phase 3)
└── README.md            # This file
```

## License

This project is licensed under the MIT License - see the LICENSE file for details.