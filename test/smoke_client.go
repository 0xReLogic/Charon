package testutil

import (
	"bufio"
	"fmt"
	"net"
	"time"
)

// RunSmokeClient dials addr, sends msg, and verifies the echoed response equals msg.
// It returns an error if any step fails.
func RunSmokeClient(addr string, msg string, timeout time.Duration) error {
	// Connect to the proxy
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return fmt.Errorf("dial error: %w", err)
	}
	defer conn.Close()

	// Send a message
	if _, err := conn.Write([]byte(msg)); err != nil {
		return fmt.Errorf("write error: %w", err)
	}

	// Read echo back
	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read error: %w", err)
	}
	fmt.Printf("Received: %s", line)

	if line != msg {
		return fmt.Errorf("mismatch: expected %q, got %q", msg, line)
	}
	return nil
}
