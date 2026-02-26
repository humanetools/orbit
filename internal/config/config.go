package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// ServiceEntry represents a service within a project topology.
type ServiceEntry struct {
	Name     string `mapstructure:"name"     yaml:"name"`
	Platform string `mapstructure:"platform" yaml:"platform"`
	ID       string `mapstructure:"id"       yaml:"id"`
}

// ProjectConfig represents a project with its service topology.
type ProjectConfig struct {
	Topology []ServiceEntry `mapstructure:"topology" yaml:"topology"`
}

// PlatformConfig holds credentials for a connected platform.
type PlatformConfig struct {
	Token string `mapstructure:"token" yaml:"token"`
}

// ThresholdConfig holds alerting thresholds.
type ThresholdConfig struct {
	ResponseTimeMs int `mapstructure:"response_time_ms" yaml:"response_time_ms"`
	CPUPercent     int `mapstructure:"cpu_percent"      yaml:"cpu_percent"`
	MemoryPercent  int `mapstructure:"memory_percent"   yaml:"memory_percent"`
}

// Config is the top-level configuration for Orbit.
type Config struct {
	DefaultProject string                   `mapstructure:"default_project" yaml:"default_project"`
	Platforms      map[string]PlatformConfig `mapstructure:"platforms"       yaml:"platforms"`
	Projects       map[string]ProjectConfig  `mapstructure:"projects"        yaml:"projects"`
	Thresholds     ThresholdConfig           `mapstructure:"thresholds"      yaml:"thresholds"`
}

// Dir returns the path to the Orbit config directory (~/.orbit/).
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".orbit"), nil
}

// EnsureDir creates ~/.orbit/ if it doesn't exist.
func EnsureDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create config dir: %w", err)
	}
	return dir, nil
}

// Load reads the config from ~/.orbit/config.yaml.
// Returns a default Config if the file doesn't exist yet.
func Load() (*Config, error) {
	dir, err := EnsureDir()
	if err != nil {
		return nil, err
	}

	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(dir)

	// Defaults
	v.SetDefault("thresholds.response_time_ms", 500)
	v.SetDefault("thresholds.cpu_percent", 80)
	v.SetDefault("thresholds.memory_percent", 85)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	// Initialize nil maps
	if cfg.Platforms == nil {
		cfg.Platforms = make(map[string]PlatformConfig)
	}
	if cfg.Projects == nil {
		cfg.Projects = make(map[string]ProjectConfig)
	}

	return &cfg, nil
}

// Save writes the config to ~/.orbit/config.yaml.
func Save(cfg *Config) error {
	dir, err := EnsureDir()
	if err != nil {
		return err
	}

	v := viper.New()
	v.SetConfigType("yaml")

	v.Set("default_project", cfg.DefaultProject)
	v.Set("platforms", cfg.Platforms)
	v.Set("projects", cfg.Projects)
	v.Set("thresholds", cfg.Thresholds)

	path := filepath.Join(dir, "config.yaml")
	return v.WriteConfigAs(path)
}
