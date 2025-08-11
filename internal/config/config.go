package config

import (
	"fmt"

	"github.com/spf13/viper"
)

// Config menyimpan konfigurasi aplikasi
type Config struct {
	ListenPort        string `mapstructure:"listen_port"`
	TargetServiceAddr string `mapstructure:"target_service_addr"`
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
