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
target_service_addr: "localhost:9091"
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

## Project Structure

```
charon/
├── cmd/
│   └── charon/          # Main application entry point
├── internal/
│   ├── config/          # Configuration handling
│   ├── proxy/           # Proxy implementation
│   └── ...
├── test/                # Test utilities and mock servers
│   ├── cmd/             # Standalone test binaries (no conflict with library)
│   │   ├── echo_server/
│   │   ├── smoke_client/
│   │   └── interactive_client/
│   ├── echo_server.go   # Library: RunEchoServer
│   ├── smoke_client.go  # Library: RunSmokeClient
│   └── test_proxy.go    # Library: RunInteractiveProxyClient
├── config.yaml          # Default configuration
└── README.md            # This file
```

## License

This project is licensed under the MIT License - see the LICENSE file for details.