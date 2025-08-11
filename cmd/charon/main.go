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

	// Create HTTP reverse proxy with per-request resolver (Phase 3)
	resolver := func(r *http.Request) (*url.URL, error) {
		// Prefer service discovery if configured
		var addr string
		if cfg.TargetServiceName != "" {
			if cfg.RegistryFile == "" {
				return nil, fmt.Errorf("registry_file is required when target_service_name is set")
			}
			a, err := registry.ResolveServiceAddress(cfg.RegistryFile, cfg.TargetServiceName)
			if err != nil {
				return nil, err
			}
			addr = a
		} else {
			addr = cfg.TargetServiceAddr
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
