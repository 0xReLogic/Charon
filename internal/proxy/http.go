package proxy

import (
	"log"
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
)

// HTTPProxy is a simple reverse proxy with basic metrics logging.
type HTTPProxy struct {
	ListenAddr string
	TargetURL  *url.URL
	// Resolver allows per-request upstream selection when set.
	Resolver func(r *http.Request) (*url.URL, error)
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

// Start runs the HTTP reverse proxy server and blocks.
func (p *HTTPProxy) Start() error {
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

	// Build reverse proxy with custom Director for dynamic target selection
	rp := &httputil.ReverseProxy{Director: func(req *http.Request) {
		var upstream *url.URL
		if p.Resolver != nil {
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
	}, Transport: transport,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			up := r.URL.Host
			if up == "" {
				up = "unknown"
			}
			log.Printf("http: proxy error upstream=%s: %v", up, err)
			http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		},
	}

	proxyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		// Resolve upstream early for consistent logging/metrics even on errors
		resolvedUp := "unknown"
		if p.Resolver != nil {
			if u, err := p.Resolver(r); err == nil && u != nil && u.Host != "" {
				resolvedUp = u.Host
			}
		} else if p.TargetURL != nil && p.TargetURL.Host != "" {
			resolvedUp = p.TargetURL.Host
		}

		rp.ServeHTTP(rec, r)
		latency := time.Since(start)

		// Log resolved upstream host for observability
		log.Printf("http request method=%s path=%s upstream=%s -> status=%d bytes=%d latency=%s", r.Method, r.URL.Path, resolvedUp, rec.status, rec.size, latency)

		// Metrics
		httpRequestsTotal.WithLabelValues(r.Method, strconv.Itoa(rec.status), resolvedUp).Inc()
		httpRequestLatency.WithLabelValues(r.Method, resolvedUp).Observe(latency.Seconds())
	})

	upstream := "dynamic"
	if p.TargetURL != nil {
		upstream = p.TargetURL.String()
	}
	log.Printf("Starting HTTP reverse proxy on %s -> %s", p.ListenAddr, upstream)

	// Serve mux: expose /metrics locally, proxy for everything else
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/", proxyHandler)
	return http.ListenAndServe(p.ListenAddr, mux)
}
