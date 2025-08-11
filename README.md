# Charon

🔱 A lightweight, high-performance service mesh sidecar proxy built with Go, designed for transparently securing and observing microservice traffic.

## Overview

Charon acts as a "bodyguard" for your microservices, intercepting all incoming and outgoing traffic, verifying identities, logging activities, and ensuring messages reach their correct destination, even if the destination moves.

## Features (Planned)

- **Transparent TCP Proxy**: Forward TCP traffic without application awareness
- **Intelligent HTTP Proxy**: Parse and understand HTTP traffic
- **Service Discovery**: Dynamically discover and connect to services
- **Secure Communication**: Automatic mTLS between services
- **Observability**: Distributed tracing and metrics collection
- **Resilience**: Circuit breaking, rate limiting, and retries

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
# Phase 3: service discovery via file registry
target_service_name: "http-backend"
registry_file: "registry.yaml"
# (Optional fallback for Phase 1/2)
# target_service_addr: "localhost:9091"
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

You can configure Prometheus to scrape `http://<charon-host>:8080/metrics`.

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