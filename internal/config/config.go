package config

import (
	"fmt"

	"github.com/spf13/viper"
)

// Config menyimpan konfigurasi aplikasi
type Config struct {
	ListenPort        string `mapstructure:"listen_port"`
	// Phase 3: gunakan nama service dan registry
	TargetServiceName string `mapstructure:"target_service_name"`
	RegistryFile      string `mapstructure:"registry_file"`
	// Backward compatibility (Phase 1/2)
	TargetServiceAddr string `mapstructure:"target_service_addr"`
	// Advanced routing rules (optional). Evaluated in order; first match wins.
	Routes            []RouteRule `mapstructure:"routes"`
}

// RouteRule mendefinisikan aturan routing berbasis host/path
type RouteRule struct {
	Host        string `mapstructure:"host"`        // optional exact host match (tanpa port)
	PathPrefix  string `mapstructure:"path_prefix"` // optional path prefix match
	ServiceName string `mapstructure:"service"`     // target service name di registry
}

// LoadConfig membaca konfigurasi dari file
func LoadConfig(path string) (*Config, error) {
	viper.SetConfigFile(path)
	viper.SetConfigType("yaml")

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	return &config, nil
}
