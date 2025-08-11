package proxy

import (
	"io"
	"log"
	"net"
	"sync"
)

// TCPProxy adalah implementasi proxy TCP sederhana
type TCPProxy struct {
	ListenAddr string
	TargetAddr string
}

// NewTCPProxy membuat instance baru TCPProxy
func NewTCPProxy(listenAddr, targetAddr string) *TCPProxy {
	return &TCPProxy{
		ListenAddr: listenAddr,
		TargetAddr: targetAddr,
	}
}

// Start memulai proxy TCP
func (p *TCPProxy) Start() error {
	listener, err := net.Listen("tcp", p.ListenAddr)
	if err != nil {
		return err
	}
	defer listener.Close()

	log.Printf("TCP Proxy listening on %s, forwarding to %s", p.ListenAddr, p.TargetAddr)

	for {
		clientConn, err := listener.Accept()
		if err != nil {
			log.Printf("Error accepting connection: %v", err)
			continue
		}

		go p.handleConnection(clientConn)
	}
}

// handleConnection menangani koneksi masuk
func (p *TCPProxy) handleConnection(clientConn net.Conn) {
	defer clientConn.Close()

	log.Printf("New connection from %s", clientConn.RemoteAddr())

	targetConn, err := net.Dial("tcp", p.TargetAddr)
	if err != nil {
		log.Printf("Error connecting to target: %v", err)
		return
	}
	defer targetConn.Close()

	// Gunakan WaitGroup untuk menunggu kedua goroutine selesai
	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine untuk menyalin data dari client ke target
	go func() {
		defer wg.Done()
		if _, err := io.Copy(targetConn, clientConn); err != nil {
			log.Printf("Error copying client -> target: %v", err)
		}
		// Tutup koneksi write ke target untuk memberi sinyal EOF
		if conn, ok := targetConn.(*net.TCPConn); ok {
			conn.CloseWrite()
		}
	}()

	// Goroutine untuk menyalin data dari target ke client
	go func() {
		defer wg.Done()
		if _, err := io.Copy(clientConn, targetConn); err != nil {
			log.Printf("Error copying target -> client: %v", err)
		}
		// Tutup koneksi write ke client untuk memberi sinyal EOF
		if conn, ok := clientConn.(*net.TCPConn); ok {
			conn.CloseWrite()
		}
	}()

	// Tunggu kedua goroutine selesai
	wg.Wait()
	log.Printf("Connection from %s closed", clientConn.RemoteAddr())
}
