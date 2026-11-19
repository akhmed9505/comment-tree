package postgres

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/akhmed9505/comment-tree/internal/config"
)

func testConfig() config.Postgres {
	return config.Postgres{
		MaxConns: 2, MinConns: 0, ConnectTimeout: time.Second,
		StatementTimeout: 1250 * time.Millisecond, LockTimeout: 275 * time.Millisecond,
	}
}

func TestParseConfig(t *testing.T) {
	for _, dsn := range []string{
		"postgres://test_user:test_password@localhost:5432/test?sslmode=disable&pool_max_conns=9&pool_min_conns=3&connect_timeout=99&statement_timeout=99&lock_timeout=99",
		"host=localhost port=5432 user=test_user password=test_password dbname=test sslmode=disable",
	} {
		cfg := testConfig()
		cfg.MinConns = 1
		parsed, err := parseConfig(dsn, cfg)
		if err != nil {
			t.Fatal(err)
		}
		if parsed.MaxConns != 2 || parsed.MinConns != 1 || parsed.ConnConfig.ConnectTimeout != time.Second {
			t.Fatal("application settings did not override DSN pool and connect settings")
		}
		params := parsed.ConnConfig.RuntimeParams
		if params["statement_timeout"] != "1250" || params["lock_timeout"] != "275" {
			t.Fatal("session timeouts must use application settings in milliseconds")
		}
		if parsed.ConnConfig.User != "test_user" || parsed.ConnConfig.Password != "test_password" {
			t.Fatal("connection credentials were not preserved")
		}
	}
}

func TestParseConfigErrorsAreSafe(t *testing.T) {
	for _, dsn := range []string{
		"postgres://private_user:private_password@localhost:%zz/test",
		"host=localhost user=private_user password=private_password port=private_port",
		"postgres://private_user:private_password@localhost/test?pool_max_conns=private_limit",
		"postgres://private_user:private_password@localhost/test?sslmode=private_mode",
	} {
		pool, err := NewPool(t.Context(), dsn, testConfig())
		if pool != nil || err == nil {
			t.Fatal("expected invalid DSN to fail before opening a pool")
		}
		if strings.Contains(err.Error(), "private_") || errors.Unwrap(err) != nil {
			t.Fatal("parse error exposed connection details")
		}
	}
}

func TestParseConfigRejectsInvalidSettings(t *testing.T) {
	for name, mutate := range map[string]func(*config.Postgres){
		"zero max": func(c *config.Postgres) { c.MaxConns = 0 },
		"oversized max": func(c *config.Postgres) {
			c.MaxConns = math.MaxInt32
			c.MaxConns++
		},
		"negative min":    func(c *config.Postgres) { c.MinConns = -1 },
		"min above max":   func(c *config.Postgres) { c.MinConns = 3 },
		"zero connect":    func(c *config.Postgres) { c.ConnectTimeout = 0 },
		"small statement": func(c *config.Postgres) { c.StatementTimeout = time.Microsecond },
		"small lock":      func(c *config.Postgres) { c.LockTimeout = time.Microsecond },
		"large statement": func(c *config.Postgres) { c.StatementTimeout = (time.Duration(math.MaxInt32) + 1) * time.Millisecond },
		"large lock":      func(c *config.Postgres) { c.LockTimeout = (time.Duration(math.MaxInt32) + 1) * time.Millisecond },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := testConfig()
			mutate(&cfg)
			if _, err := parseConfig("postgres://localhost/test?sslmode=disable", cfg); err == nil {
				t.Fatal("expected invalid settings to fail")
			}
		})
	}
	for _, params := range []string{"pool_health_check_period=0", "pool_min_idle_conns=-1", "pool_min_idle_conns=3"} {
		if _, err := parseConfig("postgres://localhost/test?"+params, testConfig()); err == nil {
			t.Fatal("expected unsafe pool settings to fail")
		}
	}
}

func TestNewPoolConnectTimeout(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	// Accept TCP without completing the PostgreSQL handshake to exercise the connect deadline.
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = io.Copy(io.Discard, conn)
	}()
	cfg := testConfig()
	cfg.ConnectTimeout = 50 * time.Millisecond
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	pool, err := NewPool(ctx, "postgres://private_user:private_password@"+listener.Addr().String()+"/test?sslmode=disable", cfg)
	if pool != nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected connect deadline, got %v", err)
	}
	if ctx.Err() != nil {
		t.Fatal("pool startup exceeded its own connect timeout")
	}
}

func TestNewPoolCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	pool, err := NewPool(ctx, "postgres://localhost/test?sslmode=disable", testConfig())
	if pool != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

func TestNewPoolServerErrorIsSafe(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	replied := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			replied <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		var length uint32
		if err := binary.Read(conn, binary.BigEndian, &length); err != nil {
			replied <- err
			return
		}
		if _, err := io.CopyN(io.Discard, conn, int64(length)-4); err != nil {
			replied <- err
			return
		}
		// PostgreSQL ErrorResponse fields can include credentials or other connection details.
		body := []byte("SFATAL\x00C28P01\x00Mprivate_server_diagnostic\x00\x00")
		frame := append([]byte{'E'}, binary.BigEndian.AppendUint32(nil, uint32(len(body)+4))...)
		_, err = conn.Write(append(frame, body...))
		replied <- err
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	pool, err := NewPool(ctx, "postgres://private_user:private_password@"+listener.Addr().String()+"/test?sslmode=disable", testConfig())
	if pool != nil {
		pool.Close()
		t.Fatal("expected server rejection")
	}
	if err == nil || strings.Contains(err.Error(), "private_") || errors.Unwrap(err) != nil {
		t.Fatal("expected credential-safe server error")
	}
	if err := <-replied; err != nil {
		t.Fatalf("send test server response: %v", err)
	}
}

func TestPingClosedPool(t *testing.T) {
	pool, err := pgxpool.New(t.Context(), "postgres://private_user:private_password@localhost/test?sslmode=disable")
	if err != nil {
		t.Fatal("create test pool failed")
	}
	pool.Close()
	err = Ping(t.Context(), pool)
	if err == nil || strings.Contains(err.Error(), "private_") || errors.Unwrap(err) != nil {
		t.Fatalf("expected safe connectivity error, got %v", err)
	}
}
