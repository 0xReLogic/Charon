package tracing

import (
	"context"
	"log"
	"net/url"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
)

const serviceName = "charon"

// InitTracing initializes OpenTelemetry tracing with service name using OTLP HTTP exporter.
func InitTracing(serviceName, jaegerEndpoint string) (func(), error) {
	// Derive OTLP endpoint from provided Jaeger endpoint (fallback to localhost:4318)
	endpoint := deriveOTLPEndpoint(jaegerEndpoint)
	// Create the OTLP HTTP exporter
	exp, err := otlptracehttp.New(context.Background(),
		otlptracehttp.WithEndpoint(endpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	tp := tracesdk.NewTracerProvider(
		// Always be sure to batch in production
		tracesdk.WithBatcher(exp),
		// Record information about this application in a Resource
		tracesdk.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion("v0.1.0"),
		)),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	// Return a shutdown function
	return func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Printf("Error shutting down tracer provider: %v", err)
		}
	}, nil
}

// Init initializes OpenTelemetry tracing
func Init(jaegerEndpoint string) (func(), error) {
	// Derive OTLP endpoint from provided Jaeger endpoint (fallback to localhost:4318)
	endpoint := deriveOTLPEndpoint(jaegerEndpoint)
	// Create the OTLP HTTP exporter
	exp, err := otlptracehttp.New(context.Background(),
		otlptracehttp.WithEndpoint(endpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	tp := tracesdk.NewTracerProvider(
		// Always be sure to batch in production
		tracesdk.WithBatcher(exp),
		// Record information about this application in a Resource
		tracesdk.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion("v0.1.0"),
		)),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	// Return a shutdown function
	return func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Printf("Error shutting down tracer provider: %v", err)
		}
	}, nil
}

// GetTracer returns the tracer for charon
func GetTracer() trace.Tracer {
	return otel.Tracer(serviceName)
}

// StartSpan starts a new span with the given name
func StartSpan(ctx context.Context, name string) (context.Context, trace.Span) {
	return GetTracer().Start(ctx, name)
}

// SpanFromContext returns the span from context
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// TraceIDFromContext extracts trace ID from context
func TraceIDFromContext(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		return span.SpanContext().TraceID().String()
	}
	return ""
}

// deriveOTLPEndpoint attempts to map a Jaeger endpoint URL to an OTLP HTTP endpoint host:port.
// If parsing fails, it defaults to "localhost:4318".
func deriveOTLPEndpoint(jaegerEndpoint string) string {
	if jaegerEndpoint == "" {
		return "localhost:4318"
	}
	if u, err := url.Parse(jaegerEndpoint); err == nil {
		hostname := u.Hostname()
		port := u.Port()
		// Map common Jaeger collector settings to OTLP default
		if port == "14268" || strings.Contains(u.Path, "/api/traces") {
			if hostname == "" {
				hostname = "localhost"
			}
			return hostname + ":4318"
		}
		if hostname == "" {
			return "localhost:4318"
		}
		if port == "" {
			// default OTLP http port
			return hostname + ":4318"
		}
		return hostname + ":" + port
	}
	return "localhost:4318"
}
