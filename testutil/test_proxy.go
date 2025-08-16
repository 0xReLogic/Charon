package testutil

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
)

// RunInteractiveProxyClient connects to addr and provides an interactive REPL that
// reads lines from stdin and writes them to the connection, printing echoed responses.
// Type 'exit' to quit. Returns an error if any IO operation fails.
func RunInteractiveProxyClient(addr string) error {
	// Connect to the proxy
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("error connecting to proxy: %w", err)
	}
	defer conn.Close()

	fmt.Printf("Connected to proxy at %s\n", addr)
	fmt.Println("Type a message and press Enter to send it. The echo server should send it back.")
	fmt.Println("Type 'exit' to quit.")

	// Create a scanner for reading from stdin
	scanner := bufio.NewScanner(os.Stdin)

	// Create a goroutine to read responses from the server
	errCh := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(conn)
		for {
			message, err := reader.ReadString('\n')
			if err != nil {
				errCh <- fmt.Errorf("error reading from server: %w", err)
				return
			}
			fmt.Printf("Received: %s", message)
		}
	}()

	// Main loop for sending messages
	for scanner.Scan() {
		select {
		case err := <-errCh:
			return err
		default:
		}
		message := scanner.Text()
		if strings.ToLower(message) == "exit" {
			break
		}

		// Send the message to the server
		if _, err := fmt.Fprintf(conn, "%s\n", message); err != nil {
			return fmt.Errorf("error sending message: %w", err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading from stdin: %w", err)
	}
	return nil
}
