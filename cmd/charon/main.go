package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/0xReLogic/Charon/internal/config"
	"github.com/0xReLogic/Charon/internal/logging"
	"github.com/0xReLogic/Charon/internal/proxy"
	"github.com/0xReLogic/Charon/internal/ratelimit"
	"github.com/0xReLogic/Charon/internal/registry"
	tlsutils "github.com/0xReLogic/Charon/internal/tls"
	"github.com/0xReLogic/Charon/internal/tracing"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

var upstreamHealth = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "charon_upstream_health",
	Help: "Upstream health status",
}, []string{"service", "upstream"})

var breakerTransitions = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "charon_circuit_breaker_transitions_total",
	Help: "Circuit breaker state transitions",
}, []string{"upstream", "to_state"})

// simple round-robin balancer with passive health (cooldown on failure)
type rrBalancer struct {
	mu        sync.Mutex
	rrIdx     map[string]int       // per-service round-robin index
	downUntil map[string]time.Time // addr -> expiry
	healthy   map[string]bool      // addr -> health
	services  map[string][]string  // service -> last seen addrs
	coolDown  time.Duration
	interval  time.Duration
	started   bool

	// circuit breaker per upstream
	cb               map[string]*cbState
	failureThreshold int
	openDuration     time.Duration
}

type cbState struct {
	state        int // 0=closed,1=open,2=half-open
	failures     int
	openUntil    time.Time
	trialAllowed bool
}

func newRRBalancer(coolDown, interval time.Duration, failureThreshold int, openDuration time.Duration) *rrBalancer {
	return &rrBalancer{rrIdx: map[string]int{}, downUntil: map[string]time.Time{}, healthy: map[string]bool{}, services: map[string][]string{}, coolDown: coolDown, interval: interval, cb: map[string]*cbState{}, failureThreshold: failureThreshold, openDuration: openDuration}
}

func (b *rrBalancer) markFailure(addr string) {
	b.mu.Lock()
	b.downUntil[addr] = time.Now().Add(b.coolDown)
	b.healthy[addr] = false
	// update health gauges for all services that include this addr
	for svc, addrs := range b.services {
		for _, a := range addrs {
			if a == addr {
				upstreamHealth.WithLabelValues(svc, addr).Set(0)
			}
		}
	}
	logging.GetLogger().Info("health_passive_down",
		zap.String("upstream", addr),
		zap.Duration("cooldown", b.coolDown),
	)

	// circuit breaker failure accounting
	now := time.Now()
	s := b.cb[addr]
	if s == nil {
		s = &cbState{}
		b.cb[addr] = s
	}
	s.failures++
	switch s.state {
	case 0: // closed
		if s.failures >= b.failureThreshold {
			s.state = 1 // open
			s.openUntil = now.Add(b.openDuration)
			s.trialAllowed = false
			logging.LogCircuitBreaker(addr, "OPEN", fmt.Sprintf("failures=%d", s.failures))
			breakerTransitions.WithLabelValues(addr, "open").Inc()
		}
	case 2: // half-open
		// failure in half-open -> go OPEN again
		s.state = 1
		s.openUntil = now.Add(b.openDuration)
		s.trialAllowed = false
		logging.LogCircuitBreaker(addr, "RE-OPEN", "half-open failure")
		breakerTransitions.WithLabelValues(addr, "open").Inc()
	}
	b.mu.Unlock()
}

func (b *rrBalancer) markSuccess(addr string) {
	b.mu.Lock()
	s := b.cb[addr]
	if s == nil {
		s = &cbState{}
		b.cb[addr] = s
	}
	s.failures = 0
	if s.state == 2 { // half-open -> close on success
		s.state = 0
		s.trialAllowed = false
		logging.LogCircuitBreaker(addr, "CLOSE", "half-open success")
		breakerTransitions.WithLabelValues(addr, "closed").Inc()
	}
	// if open and window elapsed, keep as open until selection path transitions it to half-open
	b.mu.Unlock()
}

func (b *rrBalancer) setServiceAddrs(service string, addrs []string) {
	b.mu.Lock()
	b.services[service] = append([]string(nil), addrs...)
	if !b.started {
		b.started = true
		interval := b.interval
		if interval <= 0 {
			interval = 5 * time.Second
		}
		go b.healthLoop(interval)
	}
	b.mu.Unlock()
}

func (b *rrBalancer) healthLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		// snapshot services map
		b.mu.Lock()
		snapshot := make(map[string][]string, len(b.services))
		for svc, addrs := range b.services {
			snapshot[svc] = append([]string(nil), addrs...)
		}
		b.mu.Unlock()

		for svc, addrs := range snapshot {
			for _, addr := range addrs {
				// simple TCP health check
				conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
				ok := err == nil
				if ok {
					_ = conn.Close()
				}
				b.mu.Lock()
				prev, had := b.healthy[addr]
				b.healthy[addr] = ok
				// If back healthy, clear passive cooldown early
				if ok {
					delete(b.downUntil, addr)
				}
				b.mu.Unlock()

				// update gauge and log on change or first sight
				val := 0.0
				state := "DOWN"
				if ok {
					val = 1.0
					state = "UP"
				}
				upstreamHealth.WithLabelValues(svc, addr).Set(val)
				if !had || prev != ok {
					logging.LogHealthChange(svc, addr, state)
				}
			}
		}
	}
}

func (b *rrBalancer) next(service string, addrs []string) string {
	n := len(addrs)
	if n == 0 {
		return ""
	}
	now := time.Now()
	b.mu.Lock()
	start := b.rrIdx[service]
	// First pass: prefer healthy and not in cooldown
	for i := 0; i < n; i++ {
		idx := (start + i) % n
		addr := addrs[idx]
		if until, ok := b.downUntil[addr]; ok && now.Before(until) {
			continue
		}
		// circuit breaker: handle open/half-open
		if s, ok := b.cb[addr]; ok {
			if s.state == 1 { // open
				if now.After(s.openUntil) {
					// transition to half-open, allow one trial
					s.state = 2
					s.trialAllowed = true
					logging.LogCircuitBreaker(addr, "HALF-OPEN", "open window elapsed")
					breakerTransitions.WithLabelValues(addr, "half_open").Inc()
				} else {
					continue
				}
			}
			if s.state == 2 && !s.trialAllowed {
				continue
			}
		}
		if ok, has := b.healthy[addr]; has && !ok {
			continue
		}
		b.rrIdx[service] = (idx + 1) % n
		if s, ok := b.cb[addr]; ok && s.state == 2 {
			// consume the single trial
			s.trialAllowed = false
		}
		b.mu.Unlock()
		return addr
	}
	// Second pass: allow unknown health but skip cooldown
	for i := 0; i < n; i++ {
		idx := (start + i) % n
		addr := addrs[idx]
		if until, ok := b.downUntil[addr]; ok && now.Before(until) {
			continue
		}
		if s, ok := b.cb[addr]; ok {
			if s.state == 1 {
				if now.After(s.openUntil) {
					s.state = 2
					s.trialAllowed = true
					logging.LogCircuitBreaker(addr, "HALF-OPEN", "second pass open window elapsed")
					breakerTransitions.WithLabelValues(addr, "half_open").Inc()
				} else {
					continue
				}
			}
			if s.state == 2 && !s.trialAllowed {
				continue
			}
		}
		b.rrIdx[service] = (idx + 1) % n
		if s, ok := b.cb[addr]; ok && s.state == 2 {
			s.trialAllowed = false
		}
		b.mu.Unlock()
		return addr
	}
	// All are on cooldown; pick next anyway
	pick := addrs[start%n]
	b.rrIdx[service] = (start + 1) % n
	b.mu.Unlock()
	return pick
}

func main() {
	// Parse command line flags
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	flag.Parse()

	// Load configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize structured logging
	logLevel := "info"
	if cfg.Logging.Level != "" {
		logLevel = cfg.Logging.Level
	}
	if err := logging.Init(logLevel); err != nil {
		log.Fatalf("Failed to initialize logging: %v", err)
	}
	defer func() { _ = logging.Sync() }()

	// Set environment for logger
	if cfg.Logging.Environment != "" {
		os.Setenv("CHARON_ENV", cfg.Logging.Environment)
	}

	// Initialize tracing if enabled
	if cfg.Tracing.Enabled {
		shutdown, err := tracing.InitTracing(cfg.Tracing.ServiceName, cfg.Tracing.JaegerEndpoint)
		if err != nil {
			logging.LogError("Failed to initialize tracing", map[string]interface{}{
				"error": err.Error(),
			})
		} else {
			defer shutdown()
			logging.LogInfo("Tracing initialized", map[string]interface{}{
				"service":  cfg.Tracing.ServiceName,
				"endpoint": cfg.Tracing.JaegerEndpoint,
			})
		}
	}

	// Initialize TLS certificate manager if enabled
	var certManager *tlsutils.CertManager
	if cfg.TLS.Enabled {
		var err error
		certManager, err = tlsutils.NewCertManager(cfg.TLS.CertDir)
		if err != nil {
			logging.LogError("Failed to initialize certificate manager", map[string]interface{}{
				"error":    err.Error(),
				"cert_dir": cfg.TLS.CertDir,
			})
			return
		}
		logging.LogInfo("TLS certificate manager initialized", map[string]interface{}{
			"cert_dir": cfg.TLS.CertDir,
		})
	}

	// Parse circuit breaker config with defaults
	cbThreshold := 3
	cbDuration := 20 * time.Second
	if cfg.CircuitBreaker.FailureThreshold > 0 {
		cbThreshold = cfg.CircuitBreaker.FailureThreshold
	}
	if cfg.CircuitBreaker.OpenDuration != "" {
		if d, err := time.ParseDuration(cfg.CircuitBreaker.OpenDuration); err == nil {
			cbDuration = d
		}
	}

	// init balancer (30s cooldown, 5s health interval)
	bal := newRRBalancer(30*time.Second, 5*time.Second, cbThreshold, cbDuration)

	// Create HTTP reverse proxy with per-request resolver (Phase 3 + advanced routing)
	resolver := func(r *http.Request) (*url.URL, error) {
		// Try advanced routing rules first (host/path)
		var serviceName string
		if len(cfg.Routes) > 0 {
			host := r.Host
			if i := strings.Index(host, ":"); i >= 0 { // strip port
				host = host[:i]
			}
			path := r.URL.Path
			for _, rule := range cfg.Routes {
				if rule.Host != "" && !strings.EqualFold(rule.Host, host) {
					continue
				}
				if rule.PathPrefix != "" && !strings.HasPrefix(path, rule.PathPrefix) {
					continue
				}
				serviceName = rule.ServiceName
				break
			}
		}

		// Fall back to global service name if no route matched
		if serviceName == "" && cfg.TargetServiceName != "" {
			serviceName = cfg.TargetServiceName
		}

		var addr string
		if serviceName != "" {
			if cfg.RegistryFile == "" {
				return nil, fmt.Errorf("registry_file is required when service-based routing is used")
			}
			addrs, err := registry.ResolveServiceAddresses(cfg.RegistryFile, serviceName)
			if err != nil {
				return nil, err
			}
			// update balancer's service address list for active health checks
			bal.setServiceAddrs(serviceName, addrs)
			if len(addrs) == 1 {
				addr = addrs[0]
			} else {
				addr = bal.next(serviceName, addrs)
			}
		} else {
			// Fallback to static address if configured
			addr = cfg.TargetServiceAddr
		}

		if addr == "" {
			return nil, fmt.Errorf("no upstream target resolved")
		}

		// Ensure URL has scheme - use HTTPS if upstream TLS is enabled
		if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
			if cfg.TLS.UpstreamTLS {
				addr = "https://" + addr
			} else {
				addr = "http://" + addr
			}
		}
		return url.Parse(addr)
	}

	// Setup rate limiting if configured
	var rateLimiter *ratelimit.RateLimiter
	if cfg.RateLimit.RequestsPerSecond > 0 {
		rateLimiter = ratelimit.NewRateLimiter(cfg.RateLimit.RequestsPerSecond, cfg.RateLimit.BurstSize)
		logging.LogInfo("Rate limiting initialized", map[string]interface{}{
			"rps":    cfg.RateLimit.RequestsPerSecond,
			"burst":  cfg.RateLimit.BurstSize,
			"routes": len(cfg.RateLimit.Routes),
		})
	}

	// Determine listen address for TLS
	listenAddr := ":" + cfg.ListenPort
	if cfg.TLS.Enabled && cfg.TLS.ServerPort != "" {
		listenAddr = ":" + cfg.TLS.ServerPort
	}

	httpProxy := &proxy.HTTPProxy{
		ListenAddr: listenAddr,
		Resolver:   resolver,
		OnUpstreamError: func(host string) {
			// Log upstream error for monitoring
			logging.LogInfo("Upstream error", map[string]interface{}{
				"host": host,
			})
			if host != "" {
				bal.markFailure(host)
			}
		},
		OnUpstreamSuccess: func(host string) {
			// Log upstream success for monitoring
			logging.LogInfo("Upstream success", map[string]interface{}{
				"host": host,
			})
			if host != "" {
				bal.markSuccess(host)
			}
		},
		RateLimiter:    rateLimiter,
		UseUpstreamTLS: cfg.TLS.UpstreamTLS,
	}

	// Configure TLS if enabled
	if cfg.TLS.Enabled && certManager != nil {
		httpProxy.TLSConfig = certManager.GetServerTLSConfig()
		httpProxy.ClientTLS = certManager.GetClientTLSConfig()

		logging.LogInfo("TLS configuration applied to proxy", map[string]interface{}{
			"server_tls":  true,
			"client_tls":  cfg.TLS.UpstreamTLS,
			"listen_addr": listenAddr,
		})
	}

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start proxy in a goroutine
	go func() {
		if err := httpProxy.Start(); err != nil {
			logging.GetLogger().Fatal("failed_to_start_proxy", zap.Error(err))
		}
	}()

	logging.GetLogger().Info("charon_proxy_started",
		zap.String("listen_port", cfg.ListenPort),
		zap.String("target_service", cfg.TargetServiceName),
	)

	// Wait for termination signal
	<-sigCh
	logging.GetLogger().Info("shutting_down")
}
