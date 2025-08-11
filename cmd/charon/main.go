package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/0xReLogic/Charon/internal/config"
	"github.com/0xReLogic/Charon/internal/proxy"
	"github.com/0xReLogic/Charon/internal/registry"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	flag.Parse()

	// Load configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

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
			a, err := registry.ResolveServiceAddress(cfg.RegistryFile, serviceName)
			if err != nil {
				return nil, err
			}
			addr = a
		} else {
			// Fallback to static address if configured
			addr = cfg.TargetServiceAddr
		}

		if addr == "" {
			return nil, fmt.Errorf("no upstream target resolved")
		}

		// Ensure URL has scheme
		if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
			addr = "http://" + addr
		}
		return url.Parse(addr)
	}

	httpProxy := proxy.NewHTTPProxyWithResolver(":"+cfg.ListenPort, resolver)

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start proxy in a goroutine
	go func() {
		if err := httpProxy.Start(); err != nil {
			log.Fatalf("Failed to start HTTP proxy: %v", err)
		}
	}()

	log.Printf("Charon proxy started. Press Ctrl+C to exit.")

	// Wait for termination signal
	<-sigCh
	log.Println("Shutting down...")
}
