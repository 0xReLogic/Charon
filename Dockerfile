# Build stage
FROM golang:1.21-alpine AS builder

# Install git and ca-certificates
RUN apk --no-cache add git ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -o charon ./cmd/charon

# Final stage
FROM scratch

# Copy ca-certificates from builder
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy timezone data
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy the binary
COPY --from=builder /build/charon /charon

# Create directories for configs and certs
COPY --from=builder /tmp /tmp

# Expose ports
EXPOSE 8080 8443

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/charon", "--health-check"]

# Run as non-root user
USER 65534:65534

# Set entrypoint
ENTRYPOINT ["/charon"]
CMD ["--config", "/etc/charon/config.yaml"]
