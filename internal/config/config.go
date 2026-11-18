// Package config loads and validates application configuration.
package config

import (
	"errors"
	"net"
	"strconv"
	"strings"
	"time"
)

// Config holds application settings. Connection strings are excluded from YAML.
type Config struct {
	HTTP                 HTTP     `yaml:"http"`
	Postgres             Postgres `yaml:"postgres"`
	Logging              Logging  `yaml:"logging"`
	Limits               Limits   `yaml:"limits"`
	DatabaseURL          string   `yaml:"-"`
	MigrationDatabaseURL string   `yaml:"-"`
}

// HTTP configures the HTTP server.
type HTTP struct {
	Addr              string        `yaml:"addr"`
	ReadHeaderTimeout time.Duration `yaml:"read_header_timeout"`
	ReadTimeout       time.Duration `yaml:"read_timeout"`
	WriteTimeout      time.Duration `yaml:"write_timeout"`
	IdleTimeout       time.Duration `yaml:"idle_timeout"`
	ShutdownTimeout   time.Duration `yaml:"shutdown_timeout"`
	MaxHeaderBytes    int           `yaml:"max_header_bytes"`
}

// Postgres configures the connection pool and database timeouts.
type Postgres struct {
	MaxConns         int           `yaml:"max_conns"`
	MinConns         int           `yaml:"min_conns"`
	ConnectTimeout   time.Duration `yaml:"connect_timeout"`
	StatementTimeout time.Duration `yaml:"statement_timeout"`
	LockTimeout      time.Duration `yaml:"lock_timeout"`
}

// Logging configures structured log output.
type Logging struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// Limits bounds API resource consumption. Sizes are in bytes.
type Limits struct {
	RequestTimeout        time.Duration `yaml:"request_timeout"`
	MaxRequestBytes       int64         `yaml:"max_request_bytes"`
	MaxResponseBytes      int64         `yaml:"max_response_bytes"`
	MaxResultNodes        int           `yaml:"max_result_nodes"`
	MaxConcurrentRequests int           `yaml:"max_concurrent_requests"`
}

func defaults() Config {
	return Config{
		HTTP: HTTP{
			Addr: "127.0.0.1:8080", ReadHeaderTimeout: 3 * time.Second,
			ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second,
			IdleTimeout: 60 * time.Second, ShutdownTimeout: 10 * time.Second,
			MaxHeaderBytes: 16 << 10,
		},
		Postgres: Postgres{
			MaxConns: 10, MinConns: 2, ConnectTimeout: 3 * time.Second,
			StatementTimeout: 4 * time.Second, LockTimeout: time.Second,
		},
		Logging: Logging{Level: "info", Format: "json"},
		Limits: Limits{
			RequestTimeout: 5 * time.Second, MaxRequestBytes: 64 << 10,
			MaxResponseBytes: 8 << 20, MaxResultNodes: 10_000,
			MaxConcurrentRequests: 8,
		},
	}
}

func (c Config) validate() error {
	var errs []error
	check := func(ok bool, message string) {
		if !ok {
			errs = append(errs, errors.New(message))
		}
	}
	host, port, err := net.SplitHostPort(c.HTTP.Addr)
	portNumber, portErr := strconv.Atoi(port)
	check(err == nil && portErr == nil && portNumber > 0 && portNumber <= 65535 &&
		!strings.ContainsAny(host, " \t\r\n/\\?#@"),
		"http.addr must be a host:port address with port in 1..65535")
	for _, field := range []struct {
		name  string
		value time.Duration
	}{
		{"http.read_header_timeout", c.HTTP.ReadHeaderTimeout},
		{"http.read_timeout", c.HTTP.ReadTimeout},
		{"http.write_timeout", c.HTTP.WriteTimeout},
		{"http.idle_timeout", c.HTTP.IdleTimeout},
		{"http.shutdown_timeout", c.HTTP.ShutdownTimeout},
		{"postgres.connect_timeout", c.Postgres.ConnectTimeout},
		{"postgres.statement_timeout", c.Postgres.StatementTimeout},
		{"postgres.lock_timeout", c.Postgres.LockTimeout},
		{"limits.request_timeout", c.Limits.RequestTimeout},
	} {
		check(field.value > 0, field.name+" must be positive")
	}
	check(c.HTTP.ReadHeaderTimeout <= c.HTTP.ReadTimeout,
		"http.read_header_timeout must not exceed http.read_timeout")
	check(c.Postgres.LockTimeout < c.Postgres.StatementTimeout,
		"postgres.lock_timeout must be less than postgres.statement_timeout")
	check(c.Postgres.LockTimeout >= time.Millisecond && c.Postgres.StatementTimeout >= time.Millisecond,
		"postgres.lock_timeout and postgres.statement_timeout must be at least 1ms")
	check(c.Postgres.StatementTimeout < c.Limits.RequestTimeout,
		"postgres.statement_timeout must be less than limits.request_timeout")
	check(c.HTTP.WriteTimeout > c.Limits.RequestTimeout,
		"http.write_timeout must exceed limits.request_timeout")
	check(c.HTTP.MaxHeaderBytes > 0, "http.max_header_bytes must be positive")
	check(c.Postgres.MaxConns > 0 && c.Postgres.MaxConns <= 2147483647,
		"postgres.max_conns must be in 1..2147483647")
	check(c.Postgres.MinConns >= 0 && c.Postgres.MinConns <= c.Postgres.MaxConns,
		"postgres.min_conns must be in 0..postgres.max_conns")
	check(c.Limits.MaxRequestBytes > 0, "limits.max_request_bytes must be positive")
	check(c.Limits.MaxResponseBytes > 0, "limits.max_response_bytes must be positive")
	check(c.Limits.MaxResultNodes > 0, "limits.max_result_nodes must be positive")
	check(c.Limits.MaxConcurrentRequests > 0, "limits.max_concurrent_requests must be positive")
	switch c.Logging.Level {
	case "debug", "info", "warn", "error":
	default:
		check(false, "logging.level must be debug, info, warn, or error")
	}
	check(c.Logging.Format == "json" || c.Logging.Format == "text",
		"logging.format must be json or text")
	check(strings.TrimSpace(c.DatabaseURL) != "", "APP_DATABASE_URL is required")
	return errors.Join(errs...)
}
