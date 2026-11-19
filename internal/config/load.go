package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	"go.yaml.in/yaml/v3"
)

// Load returns validated configuration with precedence defaults < YAML < .env < ENV.
// It requires YAML, allows a missing .env, and leaves the process environment unchanged.
func Load() (Config, error) {
	return load(".env", os.LookupEnv)
}

func load(envPath string, lookupEnv func(string) (string, bool)) (Config, error) {
	values := map[string]string{}
	data, err := os.ReadFile(envPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("read .env file %q: %w", envPath, err)
	}
	if err == nil {
		values, err = godotenv.Unmarshal(string(data))
		if err != nil {
			// Parser errors are omitted because they may include credentials.
			return Config{}, fmt.Errorf("parse .env file %q: invalid dotenv syntax", envPath)
		}
	}
	lookup := func(key string) (string, bool) {
		if value, ok := lookupEnv(key); ok {
			return value, true
		}
		value, ok := values[key]
		return value, ok
	}
	path := "configs/config.yaml"
	if value, ok := lookup("APP_CONFIG_PATH"); ok {
		if value == "" {
			return Config{}, errors.New("APP_CONFIG_PATH must not be empty")
		}
		path = value
	}
	cfg := defaults()
	if err := loadYAML(path, &cfg); err != nil {
		return Config{}, err
	}
	if err := applyEnvironment(&cfg, lookup); err != nil {
		return Config{}, err
	}
	if err := cfg.validate(); err != nil {
		return Config{}, fmt.Errorf("validate configuration: %w", err)
	}
	return cfg, nil
}

func loadYAML(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read YAML file %q: %w", path, err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		// Parser errors are omitted because they may include credentials.
		return fmt.Errorf("decode YAML file %q: invalid YAML syntax or empty document", path)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return fmt.Errorf("decode YAML file %q: expected exactly one YAML document", path)
	}
	if err := rejectNull(document); err != nil {
		return fmt.Errorf("decode YAML file %q: %w", path, err)
	}
	// The separate node pass rejects null, which would otherwise retain defaults.
	decoder = yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(cfg); err != nil {
		return fmt.Errorf("decode YAML file %q: invalid fields or value types; check unknown fields, duplicate keys, and field types", path)
	}
	return nil
}

func rejectNull(node yaml.Node) error {
	if node.Tag == "!!null" {
		return fmt.Errorf("line %d: null values are not allowed; omit the field to use its default", node.Line)
	}
	for _, child := range node.Content {
		if err := rejectNull(*child); err != nil {
			return err
		}
	}
	return nil
}

func applyEnvironment(cfg *Config, lookup func(string) (string, bool)) error {
	bindings := []struct {
		name string
		set  func(string) error
	}{
		{"APP_DATABASE_URL", stringValue(&cfg.DatabaseURL)},
		{"APP_HTTP_ADDR", stringValue(&cfg.HTTP.Addr)},
		{"APP_HTTP_READ_HEADER_TIMEOUT", durationValue(&cfg.HTTP.ReadHeaderTimeout)},
		{"APP_HTTP_READ_TIMEOUT", durationValue(&cfg.HTTP.ReadTimeout)},
		{"APP_HTTP_WRITE_TIMEOUT", durationValue(&cfg.HTTP.WriteTimeout)},
		{"APP_HTTP_IDLE_TIMEOUT", durationValue(&cfg.HTTP.IdleTimeout)},
		{"APP_HTTP_SHUTDOWN_TIMEOUT", durationValue(&cfg.HTTP.ShutdownTimeout)},
		{"APP_HTTP_MAX_HEADER_BYTES", intValue(&cfg.HTTP.MaxHeaderBytes)},
		{"APP_POSTGRES_MAX_CONNS", intValue(&cfg.Postgres.MaxConns)},
		{"APP_POSTGRES_MIN_CONNS", intValue(&cfg.Postgres.MinConns)},
		{"APP_POSTGRES_CONNECT_TIMEOUT", durationValue(&cfg.Postgres.ConnectTimeout)},
		{"APP_POSTGRES_STATEMENT_TIMEOUT", durationValue(&cfg.Postgres.StatementTimeout)},
		{"APP_POSTGRES_LOCK_TIMEOUT", durationValue(&cfg.Postgres.LockTimeout)},
		{"APP_LOGGING_LEVEL", stringValue(&cfg.Logging.Level)},
		{"APP_LOGGING_FORMAT", stringValue(&cfg.Logging.Format)},
		{"APP_LIMITS_REQUEST_TIMEOUT", durationValue(&cfg.Limits.RequestTimeout)},
		{"APP_LIMITS_MAX_REQUEST_BYTES", int64Value(&cfg.Limits.MaxRequestBytes)},
		{"APP_LIMITS_MAX_RESPONSE_BYTES", int64Value(&cfg.Limits.MaxResponseBytes)},
		{"APP_LIMITS_MAX_RESULT_NODES", intValue(&cfg.Limits.MaxResultNodes)},
		{"APP_LIMITS_MAX_CONCURRENT_REQUESTS", intValue(&cfg.Limits.MaxConcurrentRequests)},
	}
	for _, binding := range bindings {
		if value, ok := lookup(binding.name); ok {
			if value == "" {
				return fmt.Errorf("%s must not be empty", binding.name)
			}
			if err := binding.set(value); err != nil {
				return fmt.Errorf("%s: %w", binding.name, err)
			}
		}
	}
	return nil
}

func stringValue(target *string) func(string) error {
	return func(value string) error {
		*target = value
		return nil
	}
}

func durationValue(target *time.Duration) func(string) error {
	return func(value string) error {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return errors.New("must be a valid duration such as 5s")
		}
		*target = parsed
		return nil
	}
}

func intValue(target *int) func(string) error {
	return func(value string) error {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return errors.New("must be an integer within the supported range")
		}
		*target = parsed
		return nil
	}
}

func int64Value(target *int64) func(string) error {
	return func(value string) error {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return errors.New("must be an integer within the supported range")
		}
		*target = parsed
		return nil
	}
}
