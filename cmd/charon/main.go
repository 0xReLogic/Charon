package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/0xReLogic/Charon/internal/config"
	"github.com/0xReLogic/Charon/internal/proxy"
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

	// Create HTTP reverse proxy (Phase 2)
	httpProxy, err := proxy.NewHTTPProxy(":"+cfg.ListenPort, cfg.TargetServiceAddr)
	if err != nil {
		log.Fatalf("Failed to create HTTP proxy: %v", err)
	}

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
