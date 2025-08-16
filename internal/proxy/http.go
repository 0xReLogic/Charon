package proxy

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/0xReLogic/Charon/internal/logging"
	"github.com/0xReLogic/Charon/internal/ratelimit"
	"github.com/0xReLogic/Charon/internal/tracing"
)

// context key for chosen upstream URL
type ctxKey int

const upstreamKey ctxKey = 0

// HTTPProxy is a simple reverse proxy with basic metrics logging.
type HTTPProxy struct {
	ListenAddr string
	// Resolver resolves incoming requests to upstream URLs
	Resolver func(r *http.Request) (*url.URL, error)
	// Optional fallback target URL
	TargetURL *url.URL
	// Optional callbacks
	OnUpstreamError   func(host string)
	OnUpstreamSuccess func(host string)
	// Rate limiter
	RateLimiter *ratelimit.RateLimiter
	// TLS configuration
	TLSConfig   *tls.Config
	ClientTLS   *tls.Config
	UseUpstreamTLS bool
}

var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "charon_http_requests_total",
			Help: "Total number of HTTP requests handled by Charon",
		},
		[]string{"method", "status", "upstream"},
	)
	httpRequestLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "charon_http_request_latency_seconds",
			Help:    "Latency of HTTP requests handled by Charon",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "upstream"},
	)
	httpRetriesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "charon_http_retries_total",
			Help: "Total number of HTTP retries performed by Charon",
		},
		[]string{"method"},
	)
	httpRateLimitedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "charon_http_rate_limited_total",
			Help: "Total number of HTTP requests rate limited by Charon",
		},
		[]string{"route"},
	)
)

// NewHTTPProxy creates a new HTTP reverse proxy. target can be a full URL or host:port.
func NewHTTPProxy(listenAddr, target string) (*HTTPProxy, error) {
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		target = "http://" + target
	}
	u, err := url.Parse(target)
	if err != nil {
		return nil, err
	}
	return &HTTPProxy{ListenAddr: listenAddr, TargetURL: u}, nil
}

// NewHTTPProxyWithResolver creates a proxy that resolves the upstream per request.
func NewHTTPProxyWithResolver(listenAddr string, resolver func(r *http.Request) (*url.URL, error)) *HTTPProxy {
	return &HTTPProxy{ListenAddr: listenAddr, Resolver: resolver}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	size   int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	r.size += n
	return n, err
}

type retryTransport struct {
	base            http.RoundTripper
	maxRetries      int
	idempotentOnly  bool
	backoffFunc     func(int) time.Duration
	onRetryCallback func(method string)
}

func (rt *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error
	retries := 0
	for {
		resp, err = rt.base.RoundTrip(req)
		if err == nil || retries >= rt.maxRetries || !rt.isIdempotent(req.Method) {
			break
		}
		rt.onRetryCallback(req.Method)
		retries++
		time.Sleep(rt.backoffFunc(retries))
	}
	return resp, err
}

func (rt *retryTransport) isIdempotent(method string) bool {
	if !rt.idempotentOnly {
		return true
	}
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete:
		return true
	default:
		return false
	}
}

// createReverseProxy creates the reverse proxy with TLS support
func (p *HTTPProxy) createReverseProxy() *httputil.ReverseProxy {
	// Configure transport with sane timeouts and connection pooling
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
	}

	// Apply client TLS config if configured
	if p.UseUpstreamTLS && p.ClientTLS != nil {
		transport.TLSClientConfig = p.ClientTLS
	}

	// Wrap with a retrying transport for idempotent methods
	rt := &retryTransport{
		base:            transport,
		maxRetries:      2,
		idempotentOnly:  true,
		backoffFunc:     func(i int) time.Duration { return time.Duration(1<<i) * 150 * time.Millisecond },
		onRetryCallback: func(method string) { httpRetriesTotal.WithLabelValues(method).Inc() },
	}

	// Build reverse proxy with custom Director. We expect the handler to resolve upstream
	// and attach it to the context to avoid double-resolve inconsistencies (e.g. RR).
	rp := &httputil.ReverseProxy{Director: func(req *http.Request) {
		var upstream *url.URL
		if v := req.Context().Value(upstreamKey); v != nil {
			if u, ok := v.(*url.URL); ok {
				upstream = u
			}
		}
		if upstream == nil && p.Resolver != nil {
			if u, err := p.Resolver(req); err == nil && u != nil {
				upstream = u
			}
		}
		if upstream == nil {
			upstream = p.TargetURL
		}
		if upstream == nil {
			// No upstream; leave request as-is (will fail), but avoid panic
			return
		}
		scheme := upstream.Scheme
		if scheme == "" {
			scheme = "http"
		}
		req.URL.Scheme = scheme
		req.URL.Host = upstream.Host
		// Preserve incoming path/query; set Host header to upstream host
		req.Host = upstream.Host
	}, Transport: rt,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			up := "unknown"
			if upURL := r.Context().Value(upstreamKey); upURL != nil {
				up = upURL.(*url.URL).Host
			}
			logging.LogUpstreamError(r.Context(), up, err)
			if p.OnUpstreamError != nil && up != "" && up != "unknown" {
				p.OnUpstreamError(up)
			}
			http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		},
	}

	return rp
}

// Start starts the HTTP proxy server
func (p *HTTPProxy) Start() error {
	// Create reverse proxy
	rp := p.createReverseProxy()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Create span for tracing
		ctx, span := tracing.StartSpan(r.Context(), "http_request")
		defer span.End()

		// Set basic span attributes
		span.SetAttributes(
			attribute.String("http.method", r.Method),
			attribute.String("http.url", r.URL.String()),
			attribute.String("http.user_agent", r.UserAgent()),
		)

		r = r.WithContext(ctx)

		// Rate limiting check
		if p.RateLimiter != nil {
			route := r.URL.Path
			if !p.RateLimiter.Allow(route) {
				httpRateLimitedTotal.WithLabelValues(route).Inc()
				logging.LogRateLimited(ctx, route)
				http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
				return
			}
		}

		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		// Resolve upstream early for consistent logging/metrics and attach to context
		resolvedUp := "unknown"
		var chosen *url.URL
		if p.Resolver != nil {
			if u, err := p.Resolver(r); err == nil && u != nil && u.Host != "" {
				chosen = u
				resolvedUp = u.Host
				// Update scheme to https if upstream TLS is enabled
				if p.UseUpstreamTLS {
					chosen.Scheme = "https"
				}
			}
		}

		if chosen != nil {
			r = r.Clone(context.WithValue(r.Context(), upstreamKey, chosen))
		}

		// Add upstream information to span
		span.SetAttributes(
			attribute.String("upstream.host", resolvedUp),
		)

		rp.ServeHTTP(rec, r)
		latency := time.Since(start)

		// Set final span attributes
		span.SetAttributes(
			attribute.Int("http.status_code", rec.status),
			attribute.Int64("http.response.size", int64(rec.size)),
			attribute.Float64("http.duration_ms", float64(latency.Milliseconds())),
		)

		// Set span status based on response
		if rec.status >= 400 {
			span.SetStatus(codes.Error, http.StatusText(rec.status))
		} else {
			span.SetStatus(codes.Ok, "")
		}

		// Log HTTP request with structured logging
		logging.LogHTTPRequest(r.Context(), r.Method, r.URL.Path, resolvedUp, strconv.Itoa(rec.status), latency.Milliseconds(), int64(rec.size))

		// Count server-side errors (>=500) as upstream errors for circuit breaker, but avoid double-counting 502 from ErrorHandler
		if p.OnUpstreamError != nil && resolvedUp != "unknown" && rec.status >= 500 && rec.status != http.StatusBadGateway {
			p.OnUpstreamError(resolvedUp)
		}

		// Notify success path for circuit breaker if applicable
		if p.OnUpstreamSuccess != nil && resolvedUp != "unknown" && rec.status < 500 {
			p.OnUpstreamSuccess(resolvedUp)
		}

		// Metrics
		httpRequestsTotal.WithLabelValues(r.Method, strconv.Itoa(rec.status), resolvedUp).Inc()
		httpRequestLatency.WithLabelValues(r.Method, resolvedUp).Observe(latency.Seconds())
	})
	
	mux.Handle("/metrics", promhttp.Handler())

	server := &http.Server{
		Addr:    p.ListenAddr,
		Handler: mux,
	}

	logging.LogHTTPServerStart(p.ListenAddr)

	// Start with TLS if configured
	if p.TLSConfig != nil {
		server.TLSConfig = p.TLSConfig
		logging.LogInfo("Starting HTTPS server with mTLS", map[string]interface{}{
			"address": p.ListenAddr,
			"tls": true,
		})
		return server.ListenAndServeTLS("", "") // certificates in TLSConfig
	}

	return server.ListenAndServe()
}
