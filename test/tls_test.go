package test

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	tlsutils "github.com/0xReLogic/Charon/internal/tls"
)

func TestTLSCertificateGeneration(t *testing.T) {
	// Create temporary directory for certificates
	tempDir := t.TempDir()
	
	// Initialize certificate manager
	certManager, err := tlsutils.NewCertManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create cert manager: %v", err)
	}

	// Test server TLS config
	serverConfig := certManager.GetServerTLSConfig()
	if serverConfig == nil {
		t.Fatal("Server TLS config is nil")
	}
	
	if len(serverConfig.Certificates) == 0 {
		t.Fatal("No server certificates configured")
	}

	// Test client TLS config
	clientConfig := certManager.GetClientTLSConfig()
	if clientConfig == nil {
		t.Fatal("Client TLS config is nil")
	}
	
	if len(clientConfig.Certificates) == 0 {
		t.Fatal("No client certificates configured")
	}

	// Verify certificate files exist
	expectedFiles := []string{"ca-cert.pem", "ca-key.pem", "server-cert.pem", "server-key.pem", "client-cert.pem", "client-key.pem"}
	for _, filename := range expectedFiles {
		path := fmt.Sprintf("%s/%s", tempDir, filename)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected certificate file %s does not exist", filename)
		}
	}
}

func TestMTLSConnection(t *testing.T) {
	// Skip if running in CI without network access
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}

	// Create temporary directory for certificates
	tempDir := t.TempDir()
	
	// Initialize certificate manager
	certManager, err := tlsutils.NewCertManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create cert manager: %v", err)
	}

	// Set up test HTTPS server
	serverConfig := certManager.GetServerTLSConfig()
	
	mux := http.NewServeMux()
	mux.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("mTLS test successful"))
	})

	server := &http.Server{
		Addr:      ":0", // Let OS assign port
		Handler:   mux,
		TLSConfig: serverConfig,
	}

	// Start server in goroutine
	listener, err := tls.Listen("tcp", ":0", serverConfig)
	if err != nil {
		t.Fatalf("Failed to create TLS listener: %v", err)
	}
	defer listener.Close()

	serverAddr := listener.Addr().String()
	
	go func() {
		server.Serve(listener)
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Test client connection with mTLS
	clientConfig := certManager.GetClientTLSConfig()
	clientConfig.ServerName = "localhost" // Override for test
	
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: clientConfig,
		},
		Timeout: 5 * time.Second,
	}

	// Make request to test server
	resp, err := client.Get(fmt.Sprintf("https://%s/test", serverAddr))
	if err != nil {
		t.Fatalf("mTLS client request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	expected := "mTLS test successful"
	if string(body) != expected {
		t.Errorf("Expected body '%s', got '%s'", expected, string(body))
	}

	t.Logf("mTLS test passed successfully")
}

func TestTLSConfigValidation(t *testing.T) {
	tempDir := t.TempDir()
	
	certManager, err := tlsutils.NewCertManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create cert manager: %v", err)
	}

	serverConfig := certManager.GetServerTLSConfig()
	
	// Verify TLS version
	if serverConfig.MinVersion != tls.VersionTLS12 {
		t.Errorf("Expected minimum TLS version 1.2, got %x", serverConfig.MinVersion)
	}
	
	// Verify client authentication is required
	if serverConfig.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Errorf("Expected RequireAndVerifyClientCert, got %v", serverConfig.ClientAuth)
	}
	
	// Verify CA cert pool exists
	if serverConfig.ClientCAs == nil {
		t.Error("Client CA cert pool is nil")
	}
}
