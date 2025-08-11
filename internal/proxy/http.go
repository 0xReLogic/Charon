package proxy

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

// HTTPProxy is a simple reverse proxy with basic metrics logging.
type HTTPProxy struct {
	ListenAddr string
	TargetURL  *url.URL
	// Resolver allows per-request upstream selection when set.
	Resolver func(r *http.Request) (*url.URL, error)
}

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
	}}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		rp.ServeHTTP(rec, r)
		latency := time.Since(start)

		log.Printf("http request method=%s path=%s -> status=%d bytes=%d latency=%s", r.Method, r.URL.Path, rec.status, rec.size, latency)
	})

	upstream := "dynamic"
	if p.TargetURL != nil {
		upstream = p.TargetURL.String()
	}
	log.Printf("Starting HTTP reverse proxy on %s -> %s", p.ListenAddr, upstream)
	return http.ListenAndServe(p.ListenAddr, handler)
}
