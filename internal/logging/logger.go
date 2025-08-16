package logging

import (
	"context"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// contextKey for trace ID
type contextKey string

const TraceIDKey contextKey = "trace_id"

var logger *zap.Logger

// Init initializes the structured logger
func Init(level string) error {
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{"stdout"}
	config.ErrorOutputPaths = []string{"stderr"}

	// Set log level
	switch level {
	case "debug":
		config.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		config.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		config.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	// Development mode for better readability during development
	if os.Getenv("CHARON_ENV") == "development" {
		config.Development = true
		config.Encoding = "console"
		config.EncoderConfig = zapcore.EncoderConfig{
			TimeKey:        "time",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "caller",
			MessageKey:     "msg",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.CapitalColorLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.StringDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		}
	}

	var err error
	logger, err = config.Build()
	if err != nil {
		return err
	}

	return nil
}

// GetLogger returns the global logger instance
func GetLogger() *zap.Logger {
	if logger == nil {
		// Fallback to default production logger
		logger, _ = zap.NewProduction()
	}
	return logger
}

// WithTraceID adds trace ID to context
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, TraceIDKey, traceID)
}

// GetTraceID retrieves trace ID from context
func GetTraceID(ctx context.Context) string {
	if traceID, ok := ctx.Value(TraceIDKey).(string); ok {
		return traceID
	}
	return ""
}

// LogHTTPRequest logs HTTP request with structured fields
func LogHTTPRequest(ctx context.Context, method, path, upstream, status string, latency, size int64) {
	fields := []zap.Field{
		zap.String("method", method),
		zap.String("path", path),
		zap.String("upstream", upstream),
		zap.String("status", status),
		zap.Int64("latency_ms", latency),
		zap.Int64("size_bytes", size),
	}

	if traceID := GetTraceID(ctx); traceID != "" {
		fields = append(fields, zap.String("trace_id", traceID))
	}

	GetLogger().Info("http_request", fields...)
}

// LogUpstreamError logs upstream errors with context
func LogUpstreamError(ctx context.Context, upstream string, err error) {
	fields := []zap.Field{
		zap.String("upstream", upstream),
		zap.Error(err),
	}

	if traceID := GetTraceID(ctx); traceID != "" {
		fields = append(fields, zap.String("trace_id", traceID))
	}

	GetLogger().Error("upstream_error", fields...)
}

// LogHealthChange logs health status changes
func LogHealthChange(service, upstream, state string) {
	GetLogger().Info("health_change",
		zap.String("service", service),
		zap.String("upstream", upstream),
		zap.String("state", state),
	)
}

// LogCircuitBreaker logs circuit breaker state changes
func LogCircuitBreaker(upstream, state, reason string) {
	GetLogger().Info("circuit_breaker",
		zap.String("upstream", upstream),
		zap.String("state", state),
		zap.String("reason", reason),
	)
}

// LogRateLimited logs rate limiting events
func LogRateLimited(ctx context.Context, route string) {
	fields := []zap.Field{
		zap.String("route", route),
		zap.String("event", "rate_limited"),
	}

	if traceID := GetTraceID(ctx); traceID != "" {
		fields = append(fields, zap.String("trace_id", traceID))
	}

	GetLogger().Warn("rate_limited", fields...)
}

// LogHTTPServerStart logs HTTP server startup
func LogHTTPServerStart(addr string) {
	GetLogger().Info("http_server_start",
		zap.String("listen_addr", addr),
	)
}

// LogInfo logs general info messages with structured fields
func LogInfo(message string, fields map[string]interface{}) {
	zapFields := make([]zap.Field, 0, len(fields))
	for k, v := range fields {
		switch val := v.(type) {
		case string:
			zapFields = append(zapFields, zap.String(k, val))
		case int:
			zapFields = append(zapFields, zap.Int(k, val))
		case bool:
			zapFields = append(zapFields, zap.Bool(k, val))
		case float64:
			zapFields = append(zapFields, zap.Float64(k, val))
		default:
			zapFields = append(zapFields, zap.Any(k, v))
		}
	}
	GetLogger().Info(message, zapFields...)
}

// LogError logs error messages with structured fields
func LogError(message string, fields map[string]interface{}) {
	zapFields := make([]zap.Field, 0, len(fields))
	for k, v := range fields {
		switch val := v.(type) {
		case string:
			zapFields = append(zapFields, zap.String(k, val))
		case int:
			zapFields = append(zapFields, zap.Int(k, val))
		case bool:
			zapFields = append(zapFields, zap.Bool(k, val))
		case float64:
			zapFields = append(zapFields, zap.Float64(k, val))
		default:
			zapFields = append(zapFields, zap.Any(k, v))
		}
	}
	GetLogger().Error(message, zapFields...)
}

// Sync flushes any buffered log entries
func Sync() {
	if logger != nil {
		logger.Sync()
	}
}

// GenerateTraceID generates a simple trace ID
func GenerateTraceID() string {
	// Simple implementation - in production you'd want something more sophisticated
	return randomString(16)
}

// randomString generates a random string of given length
func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[len(charset)/2+i%len(charset)/2] // Simple deterministic pattern for demo
	}
	return string(b)
}
