package config

import (
	"fmt"

	"github.com/spf13/viper"
)

// Config menyimpan konfigurasi aplikasi
type Config struct {
	ListenPort        string `mapstructure:"listen_port"`
	// Phase 3: gunakan nama service dan registry
	TargetServiceName string `mapstructure:"target_service_name"`
	RegistryFile      string `mapstructure:"registry_file"`
	// Backward compatibility (Phase 1/2)
	TargetServiceAddr string `mapstructure:"target_service_addr"`
	// Advanced routing rules (optional). Evaluated in order; first match wins.
	Routes            []RouteRule `mapstructure:"routes"`
	// Circuit breaker configuration
	CircuitBreaker    CircuitBreakerConfig `mapstructure:"circuit_breaker"`
	// Rate limiting configuration
	RateLimit         RateLimitConfig `mapstructure:"rate_limit"`
	// Logging configuration
	Logging           LoggingConfig `mapstructure:"logging"`
	// Tracing configuration
	Tracing           TracingConfig `mapstructure:"tracing"`
	// TLS configuration
	TLS               TLSConfig `mapstructure:"tls"`
}

// RouteRule mendefinisikan aturan routing berbasis host/path
type RouteRule struct {
	Host        string `mapstructure:"host"`        // optional exact host match (tanpa port)
	PathPrefix  string `mapstructure:"path_prefix"` // optional path prefix match
	ServiceName string `mapstructure:"service"`     // target service name di registry
}

// CircuitBreakerConfig mendefinisikan konfigurasi circuit breaker
type CircuitBreakerConfig struct {
	FailureThreshold int    `mapstructure:"failure_threshold"` // consecutive failures to trip breaker
	OpenDuration     string `mapstructure:"open_duration"`     // duration to keep breaker open (e.g. "30s")
}

// RateLimitConfig mendefinisikan konfigurasi rate limiting
type RateLimitConfig struct {
	RequestsPerSecond int      `mapstructure:"requests_per_second"` // max requests per second (0 = disabled)
	BurstSize         int      `mapstructure:"burst_size"`          // max burst requests
	Routes            []string `mapstructure:"routes"`              // specific routes to apply rate limiting (empty = all routes)
}

// LoggingConfig mendefinisikan konfigurasi logging
type LoggingConfig struct {
	Level       string `mapstructure:"level"`       // log level: debug, info, warn, error
	Format      string `mapstructure:"format"`      // log format: json, console
	Environment string `mapstructure:"environment"` // environment: production, development
}

// TracingConfig mendefinisikan konfigurasi tracing
type TracingConfig struct {
	Enabled         bool   `mapstructure:"enabled"`          // enable tracing (default: false)
	JaegerEndpoint  string `mapstructure:"jaeger_endpoint"`  // Jaeger collector endpoint
	ServiceName     string `mapstructure:"service_name"`     // service name for tracing
}

// TLSConfig mendefinisikan konfigurasi TLS/mTLS
type TLSConfig struct {
	Enabled     bool   `mapstructure:"enabled"`      // enable TLS (default: false)
	CertDir     string `mapstructure:"cert_dir"`     // certificate directory
	ServerPort  string `mapstructure:"server_port"`  // HTTPS server port (if different from HTTP)
	UpstreamTLS bool   `mapstructure:"upstream_tls"` // use HTTPS for upstream connections
}

// LoadConfig membaca konfigurasi dari file
func LoadConfig(path string) (*Config, error) {
	viper.SetConfigFile(path)
	viper.SetConfigType("yaml")

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	return &config, nil
}
