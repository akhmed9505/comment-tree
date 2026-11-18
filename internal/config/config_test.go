package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const validURL = "postgres://app:secret@localhost:5432/commenttree?sslmode=disable"

func fixture(t *testing.T, yamlText, envText string) (string, map[string]string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(yamlText), 0600); err != nil {
		t.Fatal(err)
	}
	if envText != "" {
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(envText), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, ".env"), map[string]string{
		"APP_CONFIG_PATH": path, "APP_DATABASE_URL": validURL,
	}
}

func lookup(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

func TestDefaultsWithMissingDotenv(t *testing.T) {
	path, env := fixture(t, "{}", "")
	got, err := load(path, lookup(env))
	if err != nil {
		t.Fatal(err)
	}
	want := defaults()
	want.DatabaseURL = validURL
	if got != want {
		t.Fatal("configuration does not match defaults")
	}
}

func TestPrecedence(t *testing.T) {
	path, env := fixture(t, `http:
  addr: "127.0.0.1:9000"
  idle_timeout: 75s
logging:
  level: debug
`, `APP_HTTP_ADDR=127.0.0.1:9001
APP_LOGGING_LEVEL=warn
APP_LIMITS_MAX_RESULT_NODES=12000
APP_DATABASE_URL='postgres://dotenv:dotenv-password@localhost/dotenv'
`)
	env["APP_HTTP_ADDR"] = "127.0.0.1:9002"
	got, err := load(path, lookup(env))
	if err != nil {
		t.Fatal(err)
	}
	if got.HTTP.Addr != "127.0.0.1:9002" || got.HTTP.IdleTimeout != 75*time.Second ||
		got.Logging.Level != "warn" || got.Limits.MaxResultNodes != 12000 ||
		got.HTTP.ReadTimeout != 10*time.Second || got.DatabaseURL != validURL {
		t.Fatal("defaults -> YAML -> .env -> ENV precedence was not preserved")
	}
}

func TestDotenvCredentialsAndConfigPath(t *testing.T) {
	path, env := fixture(t, "logging:\n  format: text\n", "")
	content := "APP_CONFIG_PATH='" + env["APP_CONFIG_PATH"] + "'\n" +
		"APP_DATABASE_URL='" + validURL + "'\n" +
		"APP_MIGRATION_DATABASE_URL='postgresql://migrator:password@localhost/db'\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := load(path, lookup(nil))
	if err != nil {
		t.Fatal(err)
	}
	if got.DatabaseURL != validURL || got.MigrationDatabaseURL == "" || got.Logging.Format != "text" {
		t.Fatal("dotenv credentials or configuration path were not loaded")
	}
}

func TestEnvironmentOverridesInvalidDotenvValue(t *testing.T) {
	path, env := fixture(t, "{}", "APP_POSTGRES_MAX_CONNS=invalid\nAPP_CONFIG_PATH=missing.yaml\n")
	env["APP_POSTGRES_MAX_CONNS"] = "12"
	got, err := load(path, lookup(env))
	if err != nil || got.Postgres.MaxConns != 12 {
		t.Fatalf("valid environment override was not applied: %v", err)
	}
}

func TestYAMLErrors(t *testing.T) {
	for _, tc := range []struct{ name, input, want string }{
		{"empty", "", "decode YAML"},
		{"syntax", "http: [", "decode YAML"},
		{"unknown section", "htpt: {}", "invalid fields or value types"},
		{"unknown field", "http:\n  adress: x", "invalid fields or value types"},
		{"duplicate", "http:\n  addr: x\n  addr: y", "invalid fields or value types"},
		{"wrong type", "postgres:\n  max_conns: many", "decode YAML"},
		{"invalid duration", "http:\n  read_timeout: forever", "decode YAML"},
		{"multiple documents", "{}\n---\n{}", "exactly one"},
		{"null document", "null", "null values"},
		{"null section", "http: null", "null values"},
		{"null field", "http:\n  addr:", "null values"},
		{"database URL", "database_url: secret", "invalid fields or value types"},
		{"username", "postgres:\n  user: secret", "invalid fields or value types"},
		{"password", "postgres:\n  password: secret", "invalid fields or value types"},
		{"migration URL", "migration_database_url: secret", "invalid fields or value types"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, env := fixture(t, tc.input, "")
			_, err := load(path, lookup(env))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q, got %v", tc.want, err)
			}
		})
	}
}

func TestReadErrors(t *testing.T) {
	t.Run("missing YAML", func(t *testing.T) {
		path, env := fixture(t, "{}", "")
		env["APP_CONFIG_PATH"] += ".missing"
		_, err := load(path, lookup(env))
		if err == nil || !strings.Contains(err.Error(), "read YAML file") {
			t.Fatalf("expected YAML read error, got %v", err)
		}
	})
	t.Run("dotenv directory", func(t *testing.T) {
		_, err := load(t.TempDir(), lookup(nil))
		if err == nil || !strings.Contains(err.Error(), "read .env file") {
			t.Fatalf("expected dotenv read error, got %v", err)
		}
	})
	t.Run("invalid dotenv hides secrets", func(t *testing.T) {
		path, env := fixture(t, "{}", "!bad=super-secret\n")
		_, err := load(path, lookup(env))
		if err == nil || !strings.Contains(err.Error(), "invalid dotenv syntax") || strings.Contains(err.Error(), "super-secret") {
			t.Fatalf("expected sanitized dotenv syntax error, got %v", err)
		}
	})
}

func TestParseErrorsDoNotExposeInput(t *testing.T) {
	const secret = "PRIVATE_CONFIG_MARKER"
	for _, tc := range []struct {
		name string
		yaml string
		env  string
	}{
		{"YAML string in integer field", "postgres:\n  max_conns: " + secret, ""},
		{"YAML string in duration field", "http:\n  read_timeout: " + secret, ""},
		{"YAML unknown key", secret + ": value", ""},
		{"YAML duplicate key", secret + ": first\n" + secret + ": second", ""},
		{"YAML undefined alias", "http: *" + secret, ""},
		{"YAML sensitive field", "postgres:\n  password: " + secret, ""},
		{"YAML malformed second document", "{}\n---\nhttp: *" + secret, ""},
		{"dotenv invalid key", "{}", "!" + secret + "=password\n"},
		{"dotenv unclosed quote", "{}", "APP_DATABASE_URL='" + secret},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, env := fixture(t, tc.yaml, tc.env)
			_, err := load(path, lookup(env))
			if err == nil {
				t.Fatal("expected a parsing error")
			}
			for cause := err; cause != nil; cause = errors.Unwrap(cause) {
				// Parser diagnostics may truncate values; a prefix detects partial leaks.
				if strings.Contains(cause.Error(), "PRIVATE") {
					t.Fatal("parsing error exposed input content")
				}
			}
		})
	}
}

func TestEnvironmentErrors(t *testing.T) {
	for _, tc := range []struct{ key, value, want string }{
		{"APP_CONFIG_PATH", "", "APP_CONFIG_PATH"},
		{"APP_DATABASE_URL", "", "APP_DATABASE_URL"},
		{"APP_HTTP_ADDR", "", "APP_HTTP_ADDR"},
		{"APP_HTTP_READ_TIMEOUT", "forever", "valid duration"},
		{"APP_POSTGRES_MAX_CONNS", "many", "integer"},
		{"APP_LIMITS_MAX_REQUEST_BYTES", "9223372036854775808", "integer"},
		{"APP_POSTGRES_MAX_CONNS", "9223372036854775808", "integer"},
		{"APP_LIMITS_MAX_RESPONSE_BYTES", "huge", "integer"},
	} {
		t.Run(tc.key+tc.value, func(t *testing.T) {
			path, env := fixture(t, "{}", "APP_DATABASE_URL="+validURL+"\n")
			env[tc.key] = tc.value
			_, err := load(path, lookup(env))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q, got %v", tc.want, err)
			}
		})
	}
}

func TestValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*Config)
		want string
	}{
		{"missing secret", func(c *Config) { c.DatabaseURL = "" }, "APP_DATABASE_URL"},
		{"blank secret", func(c *Config) { c.DatabaseURL = " \t" }, "APP_DATABASE_URL"},
		{"bad address", func(c *Config) { c.HTTP.Addr = "localhost" }, "http.addr"},
		{"invalid host", func(c *Config) { c.HTTP.Addr = "bad host:8080" }, "http.addr"},
		{"zero port", func(c *Config) { c.HTTP.Addr = ":0" }, "http.addr"},
		{"large port", func(c *Config) { c.HTTP.Addr = ":65536" }, "http.addr"},
		{"negative timeout", func(c *Config) { c.HTTP.ReadTimeout = -1 }, "http.read_timeout must be positive"},
		{"zero timeout", func(c *Config) { c.HTTP.IdleTimeout = 0 }, "http.idle_timeout"},
		{"header timeout", func(c *Config) { c.HTTP.ReadHeaderTimeout = 11 * time.Second }, "must not exceed"},
		{"lock timeout", func(c *Config) { c.Postgres.LockTimeout = c.Postgres.StatementTimeout }, "lock_timeout must be less"},
		{"submillisecond SQL timeout", func(c *Config) { c.Postgres.LockTimeout = time.Nanosecond }, "at least 1ms"},
		{"SQL timeout", func(c *Config) { c.Postgres.StatementTimeout = c.Limits.RequestTimeout }, "statement_timeout must be less"},
		{"write timeout", func(c *Config) { c.HTTP.WriteTimeout = c.Limits.RequestTimeout }, "write_timeout must exceed"},
		{"headers", func(c *Config) { c.HTTP.MaxHeaderBytes = 0 }, "max_header_bytes"},
		{"pool size", func(c *Config) { c.Postgres.MaxConns = 0 }, "max_conns"},
		{"pool overflow", func(c *Config) { c.Postgres.MaxConns = 2147483648 }, "max_conns"},
		{"pool minimum", func(c *Config) { c.Postgres.MinConns = -1 }, "min_conns"},
		{"pool mismatch", func(c *Config) { c.Postgres.MinConns = 11 }, "min_conns"},
		{"request bytes", func(c *Config) { c.Limits.MaxRequestBytes = 0 }, "max_request_bytes"},
		{"response bytes", func(c *Config) { c.Limits.MaxResponseBytes = -1 }, "max_response_bytes"},
		{"nodes", func(c *Config) { c.Limits.MaxResultNodes = 0 }, "max_result_nodes"},
		{"concurrency", func(c *Config) { c.Limits.MaxConcurrentRequests = 0 }, "max_concurrent_requests"},
		{"log level", func(c *Config) { c.Logging.Level = "trace" }, "logging.level"},
		{"log format", func(c *Config) { c.Logging.Format = "yaml" }, "logging.format"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaults()
			cfg.DatabaseURL = validURL
			tc.edit(&cfg)
			err := cfg.validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q, got %v", tc.want, err)
			}
		})
	}
}

func TestConnectionStringsAreNotParsed(t *testing.T) {
	for _, value := range []string{
		validURL,
		"host=localhost user=app password=secret dbname=commenttree",
		"not-a-connection-string",
	} {
		path, env := fixture(t, "{}", "")
		env["APP_DATABASE_URL"] = value
		env["APP_MIGRATION_DATABASE_URL"] = value
		got, err := load(path, lookup(env))
		if err != nil {
			t.Fatalf("technical connection validation must be deferred to pgxpool: %v", err)
		}
		if got.DatabaseURL != value || got.MigrationDatabaseURL != value {
			t.Fatal("connection strings must be passed through unchanged")
		}
	}
}

func TestLoadDoesNotChangeEnvironment(t *testing.T) {
	// Ambient APP_ variables would make this test depend on the host environment.
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "APP_") {
			t.Setenv(key, "")
			if err := os.Unsetenv(key); err != nil {
				t.Fatal(err)
			}
		}
	}
	path, env := fixture(t, "{}", "APP_LOGGING_LEVEL=debug\nAPP_DATABASE_URL="+validURL+"\n")
	t.Chdir(filepath.Dir(path))
	t.Setenv("APP_CONFIG_PATH", env["APP_CONFIG_PATH"])
	t.Setenv("APP_HTTP_ADDR", "127.0.0.1:9090")
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.HTTP.Addr != "127.0.0.1:9090" || got.Logging.Level != "debug" {
		t.Fatal("public loader did not apply environment and dotenv")
	}
	if _, ok := os.LookupEnv("APP_DATABASE_URL"); ok {
		t.Fatal("dotenv modified the process environment")
	}
}

func TestProjectConfiguration(t *testing.T) {
	path, env := fixture(t, "{}", "")
	env["APP_CONFIG_PATH"] = filepath.Join("..", "..", "configs", "config.yaml")
	got, err := load(path, lookup(env))
	if err != nil {
		t.Fatal(err)
	}
	want := defaults()
	want.DatabaseURL = validURL
	if got != want {
		t.Fatal("checked-in YAML does not match the intended defaults")
	}
}
