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
	services map[string][]string
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

func loadRegistry(registryPath string) (map[string][]string, error) {
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
	// Support both string and list of strings for each service entry
	raw := v.Get("services")
	out := map[string][]string{}
	if raw != nil {
		if mp, ok := raw.(map[string]interface{}); ok {
			for k, val := range mp {
				switch vv := val.(type) {
				case string:
					if s := strings.TrimSpace(vv); s != "" {
						out[k] = []string{s}
					}
				case []interface{}:
					var list []string
					for _, it := range vv {
						if s, ok := it.(string); ok && strings.TrimSpace(s) != "" {
							list = append(list, s)
						}
					}
					if len(list) > 0 {
						out[k] = list
					}
				case []string:
					if len(vv) > 0 {
						out[k] = vv
					}
				}
			}
		}
	}

	mu.Lock()
	cache[registryPath] = &cachedRegistry{modTime: fi.ModTime(), services: out}
	mu.Unlock()

	// Start a file watcher (best-effort) to invalidate cache on change
	ensureWatcher(registryPath)

	return out, nil
}

// ResolveServiceAddress reads a YAML registry file and returns the address for a given service name.
// Expected format:
// services:
//   service-name: host:port
// ResolveServiceAddresses returns a list of addresses for a given service name.
// Each address is in host:port form.
func ResolveServiceAddresses(registryPath, serviceName string) ([]string, error) {
	m, err := loadRegistry(registryPath)
	if err != nil {
		return nil, err
	}
	addrs, ok := m[serviceName]
	if !ok || len(addrs) == 0 {
		return nil, fmt.Errorf("service %q not found in registry", serviceName)
	}
	return addrs, nil
}

// ResolveServiceAddress returns the first address for backward compatibility.
func ResolveServiceAddress(registryPath, serviceName string) (string, error) {
	addrs, err := ResolveServiceAddresses(registryPath, serviceName)
	if err != nil {
		return "", err
	}
	return addrs[0], nil
}
