// Package config provides application configuration loading and defaults.
package config

import (
	"fmt"
	"time"

	"github.com/wb-go/wbf/config"
)

// Config holds the application-wide configuration state.
type Config struct {
	Env      string   `mapstructure:"env"`
	HTTP     HTTP     `mapstructure:"http"`
	Postgres Postgres `mapstructure:"postgres"`
	Logging  Logging  `mapstructure:"logging"`
}

// HTTP defines settings for the network transport layer.
type HTTP struct {
	Port            string        `mapstructure:"port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	IdleTimeout     time.Duration `mapstructure:"idle_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

// Postgres encapsulates database connection settings.
type Postgres struct {
	ConnectionURL string `mapstructure:"connection_url"`
}

// Logging defines the verbosity and format of application logs.
type Logging struct {
	Level string `mapstructure:"level"`
}

// Load initializes the config registry, applies defaults, and loads
// configuration files and environment variables.
func Load(configPath string) (*Config, error) {
	c := config.New()

	setDefaults(c)

	_ = c.LoadEnvFiles(".env")

	if err := c.LoadConfigFiles(configPath); err != nil {
		return nil, fmt.Errorf("failed to load config file %s: %w", configPath, err)
	}

	c.EnableEnv("APP")

	var cfg Config
	if err := c.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}

// setDefaults populates the configuration with sensible default values.
func setDefaults(c *config.Config) {
	c.SetDefault("env", "development")
	c.SetDefault("http.port", ":8080")
	c.SetDefault("http.read_timeout", "10s")
	c.SetDefault("http.write_timeout", "10s")
	c.SetDefault("http.idle_timeout", "60s")
	c.SetDefault("http.shutdown_timeout", "8s")
	c.SetDefault("logging.level", "info")
}
