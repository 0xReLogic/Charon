package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	port := "9091"
	if len(os.Args) > 1 {
		port = os.Args[1]
	}

	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"healthy","port":"%s"}`, port)
	})

	// Root endpoint
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"message":"Hello from backend server","port":"%s","path":"%s","method":"%s"}`,
			port, r.URL.Path, r.Method)
	})

	// API endpoint
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"service":"backend-%s","endpoint":"%s","timestamp":"%s"}`,
			port, r.URL.Path, "2025-08-16T18:08:50Z")
	})

	addr := ":" + port
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Setup graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		fmt.Printf("Starting backend server on http://localhost%s\n", addr)
		fmt.Printf("Health check: http://localhost%s/health\n", addr)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	<-stop
	fmt.Printf("\nShutting down backend server on port %s...\n", port)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server shutdown failed: %v", err)
	}
	fmt.Printf("Backend server on port %s stopped\n", port)
}
