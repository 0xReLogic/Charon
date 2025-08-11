package registry

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// ResolveServiceAddress reads a YAML registry file and returns the address for a given service name.
// Expected format:
// services:
//   service-name: host:port
func ResolveServiceAddress(registryPath, serviceName string) (string, error) {
	v := viper.New()
	v.SetConfigFile(registryPath)
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		return "", fmt.Errorf("read registry: %w", err)
	}
	m := v.GetStringMapString("services")
	addr, ok := m[serviceName]
	if !ok || strings.TrimSpace(addr) == "" {
		return "", fmt.Errorf("service %q not found in registry", serviceName)
	}
	return addr, nil
}
