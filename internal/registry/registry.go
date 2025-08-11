package registry

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
)

// simple in-memory cache keyed by registry path, refreshed when file mtime changes
var (
	mu    sync.RWMutex
	cache = map[string]*cachedRegistry{}
)

type cachedRegistry struct {
	modTime  time.Time
	services map[string]string
}

func loadRegistry(registryPath string) (map[string]string, error) {
	fi, err := os.Stat(registryPath)
	if err != nil {
		return nil, fmt.Errorf("stat registry: %w", err)
	}

	mu.RLock()
	if ce, ok := cache[registryPath]; ok && ce.modTime.Equal(fi.ModTime()) {
		services := ce.services
		mu.RUnlock()
		return services, nil
	}
	mu.RUnlock()

	v := viper.New()
	v.SetConfigFile(registryPath)
	v.SetConfigType("yaml")
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read registry: %w", err)
	}
	m := v.GetStringMapString("services")

	mu.Lock()
	cache[registryPath] = &cachedRegistry{modTime: fi.ModTime(), services: m}
	mu.Unlock()

	return m, nil
}

// ResolveServiceAddress reads a YAML registry file and returns the address for a given service name.
// Expected format:
// services:
//   service-name: host:port
func ResolveServiceAddress(registryPath, serviceName string) (string, error) {
	m, err := loadRegistry(registryPath)
	if err != nil {
		return "", err
	}
	addr, ok := m[serviceName]
	if !ok || strings.TrimSpace(addr) == "" {
		return "", fmt.Errorf("service %q not found in registry", serviceName)
	}
	return addr, nil
}
