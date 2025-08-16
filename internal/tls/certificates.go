package tls

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// CertManager handles certificate generation and management
type CertManager struct {
	certDir    string
	caCert     *x509.Certificate
	caKey      *rsa.PrivateKey
	serverCert tls.Certificate
	clientCert tls.Certificate
}

// NewCertManager creates a new certificate manager
func NewCertManager(certDir string) (*CertManager, error) {
	if err := os.MkdirAll(certDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cert directory: %w", err)
	}

	cm := &CertManager{certDir: certDir}
	
	// Load or generate CA
	if err := cm.setupCA(); err != nil {
		return nil, fmt.Errorf("failed to setup CA: %w", err)
	}

	// Load or generate server certificate
	if err := cm.setupServerCert(); err != nil {
		return nil, fmt.Errorf("failed to setup server cert: %w", err)
	}

	// Load or generate client certificate
	if err := cm.setupClientCert(); err != nil {
		return nil, fmt.Errorf("failed to setup client cert: %w", err)
	}

	return cm, nil
}

// setupCA loads or generates a CA certificate
func (cm *CertManager) setupCA() error {
	caKeyPath := filepath.Join(cm.certDir, "ca-key.pem")
	caCertPath := filepath.Join(cm.certDir, "ca-cert.pem")

	// Try to load existing CA
	if _, err := os.Stat(caKeyPath); err == nil {
		if _, err := os.Stat(caCertPath); err == nil {
			return cm.loadCA(caKeyPath, caCertPath)
		}
	}

	// Generate new CA
	return cm.generateCA(caKeyPath, caCertPath)
}

// loadCA loads existing CA certificate and key
func (cm *CertManager) loadCA(keyPath, certPath string) error {
	// Load CA key
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return err
	}
	keyBlock, _ := pem.Decode(keyData)
	if keyBlock == nil {
		return fmt.Errorf("failed to decode CA key")
	}
	cm.caKey, err = x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		return err
	}

	// Load CA cert
	certData, err := os.ReadFile(certPath)
	if err != nil {
		return err
	}
	certBlock, _ := pem.Decode(certData)
	if certBlock == nil {
		return fmt.Errorf("failed to decode CA cert")
	}
	cm.caCert, err = x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return err
	}

	return nil
}

// generateCA generates a new CA certificate and key
func (cm *CertManager) generateCA(keyPath, certPath string) error {
	// Generate CA key
	caKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return err
	}
	cm.caKey = caKey

	// Create CA certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{"Charon Service Mesh"},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{""},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour), // 1 year
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	// Create CA certificate
	caCertDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &caKey.PublicKey, caKey)
	if err != nil {
		return err
	}

	// Parse the certificate
	cm.caCert, err = x509.ParseCertificate(caCertDER)
	if err != nil {
		return err
	}

	// Save CA key
	keyOut, err := os.Create(keyPath)
	if err != nil {
		return err
	}
	defer keyOut.Close()
	
	keyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(caKey),
	}
	if err := pem.Encode(keyOut, keyPEM); err != nil {
		return err
	}

	// Save CA cert
	certOut, err := os.Create(certPath)
	if err != nil {
		return err
	}
	defer certOut.Close()

	certPEM := &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caCertDER,
	}
	if err := pem.Encode(certOut, certPEM); err != nil {
		return err
	}

	return nil
}

// setupServerCert loads or generates server certificate
func (cm *CertManager) setupServerCert() error {
	keyPath := filepath.Join(cm.certDir, "server-key.pem")
	certPath := filepath.Join(cm.certDir, "server-cert.pem")

	// Try to load existing cert
	if _, err := os.Stat(keyPath); err == nil {
		if _, err := os.Stat(certPath); err == nil {
			cert, err := tls.LoadX509KeyPair(certPath, keyPath)
			if err == nil {
				cm.serverCert = cert
				return nil
			}
		}
	}

	// Generate new server cert
	return cm.generateServerCert(keyPath, certPath)
}

// generateServerCert generates a new server certificate
func (cm *CertManager) generateServerCert(keyPath, certPath string) error {
	// Generate server key
	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	// Create server certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			Organization: []string{"Charon Service Mesh"},
			CommonName:   "charon-server",
		},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour), // 1 year
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		DNSNames:     []string{"localhost", "charon"},
	}

	// Create server certificate
	serverCertDER, err := x509.CreateCertificate(rand.Reader, &template, cm.caCert, &serverKey.PublicKey, cm.caKey)
	if err != nil {
		return err
	}

	// Save server key
	keyOut, err := os.Create(keyPath)
	if err != nil {
		return err
	}
	defer keyOut.Close()
	
	keyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(serverKey),
	}
	if err := pem.Encode(keyOut, keyPEM); err != nil {
		return err
	}

	// Save server cert
	certOut, err := os.Create(certPath)
	if err != nil {
		return err
	}
	defer certOut.Close()

	certPEM := &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: serverCertDER,
	}
	if err := pem.Encode(certOut, certPEM); err != nil {
		return err
	}

	// Load the certificate pair
	cm.serverCert, err = tls.LoadX509KeyPair(certPath, keyPath)
	return err
}

// setupClientCert loads or generates client certificate
func (cm *CertManager) setupClientCert() error {
	keyPath := filepath.Join(cm.certDir, "client-key.pem")
	certPath := filepath.Join(cm.certDir, "client-cert.pem")

	// Try to load existing cert
	if _, err := os.Stat(keyPath); err == nil {
		if _, err := os.Stat(certPath); err == nil {
			cert, err := tls.LoadX509KeyPair(certPath, keyPath)
			if err == nil {
				cm.clientCert = cert
				return nil
			}
		}
	}

	// Generate new client cert
	return cm.generateClientCert(keyPath, certPath)
}

// generateClientCert generates a new client certificate
func (cm *CertManager) generateClientCert(keyPath, certPath string) error {
	// Generate client key
	clientKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	// Create client certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject: pkix.Name{
			Organization: []string{"Charon Service Mesh"},
			CommonName:   "charon-client",
		},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour), // 1 year
		SubjectKeyId: []byte{1, 2, 3, 4, 7},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	// Create client certificate
	clientCertDER, err := x509.CreateCertificate(rand.Reader, &template, cm.caCert, &clientKey.PublicKey, cm.caKey)
	if err != nil {
		return err
	}

	// Save client key
	keyOut, err := os.Create(keyPath)
	if err != nil {
		return err
	}
	defer keyOut.Close()
	
	keyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(clientKey),
	}
	if err := pem.Encode(keyOut, keyPEM); err != nil {
		return err
	}

	// Save client cert
	certOut, err := os.Create(certPath)
	if err != nil {
		return err
	}
	defer certOut.Close()

	certPEM := &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: clientCertDER,
	}
	if err := pem.Encode(certOut, certPEM); err != nil {
		return err
	}

	// Load the certificate pair
	cm.clientCert, err = tls.LoadX509KeyPair(certPath, keyPath)
	return err
}

// GetServerTLSConfig returns TLS config for server
func (cm *CertManager) GetServerTLSConfig() *tls.Config {
	caCertPool := x509.NewCertPool()
	caCertPool.AddCert(cm.caCert)

	return &tls.Config{
		Certificates: []tls.Certificate{cm.serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caCertPool,
		MinVersion:   tls.VersionTLS12,
	}
}

// GetClientTLSConfig returns TLS config for client
func (cm *CertManager) GetClientTLSConfig() *tls.Config {
	caCertPool := x509.NewCertPool()
	caCertPool.AddCert(cm.caCert)

	return &tls.Config{
		Certificates: []tls.Certificate{cm.clientCert},
		RootCAs:      caCertPool,
		MinVersion:   tls.VersionTLS12,
		ServerName:   "charon-server", // Must match server cert CommonName
	}
}
