package registry

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

// simple in-memory cache keyed by registry path, refreshed when file mtime changes
var (
	mu    sync.RWMutex
	cache = map[string]*cachedRegistry{}
	watch = map[string]*fsnotify.Watcher{}
)

type cachedRegistry struct {
	modTime  time.Time
	services map[string]string
}

// ensureWatcher starts a file watcher for the given registry path (idempotent).
func ensureWatcher(registryPath string) {
	mu.Lock()
	if _, ok := watch[registryPath]; ok {
		mu.Unlock()
		return
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		// best-effort; skip watcher if cannot create
		mu.Unlock()
		return
	}
	if err := w.Add(registryPath); err != nil {
		// skip watcher if cannot add file (may not exist yet)
		_ = w.Close()
		mu.Unlock()
		return
	}
	watch[registryPath] = w
	mu.Unlock()

	// Invalidate cache on any fs event; reload will occur on next Resolve
	go func() {
		for {
			select {
			case _, ok := <-w.Events:
				if !ok {
					return
				}
				mu.Lock()
				delete(cache, registryPath)
				mu.Unlock()
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
				// ignore errors; cache will refresh on next access
			}
		}
	}()
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

	// Start a file watcher (best-effort) to invalidate cache on change
	ensureWatcher(registryPath)

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
