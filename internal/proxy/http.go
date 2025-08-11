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
	rp := httputil.NewSingleHostReverseProxy(p.TargetURL)

	// Wrap the Director to preserve the incoming request path and host
	origDirector := rp.Director
	rp.Director = func(req *http.Request) {
		origDirector(req)
		// Keep original host for upstream if needed
		req.Host = p.TargetURL.Host
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		rp.ServeHTTP(rec, r)
		latency := time.Since(start)

		log.Printf("http request method=%s path=%s -> status=%d bytes=%d latency=%s", r.Method, r.URL.Path, rec.status, rec.size, latency)
	})

	log.Printf("Starting HTTP reverse proxy on %s -> %s", p.ListenAddr, p.TargetURL.String())
	return http.ListenAndServe(p.ListenAddr, handler)
}
