package testutil

import (
	"io"
	"log"
	"net"
)

// RunEchoServer starts an echo server on the given port and blocks.
func RunEchoServer(port string) error {
	// Create TCP listener
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return err
	}
	defer listener.Close()

	log.Printf("Echo server listening on :%s", port)

	// Accept and handle connections
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Failed to accept connection: %v", err)
			continue
		}

		go handleEchoConnection(conn)
	}
}

func handleEchoConnection(conn net.Conn) {
	defer conn.Close()

	log.Printf("New connection from %s", conn.RemoteAddr())

	// Buat buffer untuk membaca data
	buffer := make([]byte, 1024)
	for {
		// Baca data dari koneksi
		n, err := conn.Read(buffer)
		if err != nil {
			if err != io.EOF {
				log.Printf("Error reading data: %v", err)
			}
			break
		}

		// Log data yang diterima
		data := buffer[:n]
		log.Printf("Received data: %s", string(data))

		// Kirim kembali data yang sama
		_, err = conn.Write(data)
		if err != nil {
			log.Printf("Error writing data: %v", err)
			break
		}
	}

	log.Printf("Connection from %s closed", conn.RemoteAddr())
}
