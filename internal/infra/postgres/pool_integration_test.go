//go:build integration

package postgres

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/akhmed9505/comment-tree/internal/testutil"
)

func TestPoolIntegration(t *testing.T) {
	dsn := testutil.StartPostgres(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	cfg := testConfig()
	cfg.MinConns = 1
	cfg.ConnectTimeout = 5 * time.Second
	cfg.StatementTimeout = 300 * time.Millisecond
	cfg.LockTimeout = 75 * time.Millisecond
	pool, err := NewPool(ctx, dsn, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := Ping(ctx, pool); err != nil {
		t.Fatal(err)
	}

	t.Run("session settings and pool limit", func(t *testing.T) {
		var connections []*pgxpool.Conn
		defer func() {
			for _, conn := range connections {
				conn.Release()
			}
		}()
		for range cfg.MaxConns {
			conn, err := pool.Acquire(ctx)
			if err != nil {
				t.Fatal("acquire test connection failed")
			}
			connections = append(connections, conn)
			var statement, lock string
			err = conn.QueryRow(ctx, "SELECT current_setting('statement_timeout'), current_setting('lock_timeout')").Scan(&statement, &lock)
			if err != nil || statement != "300ms" || lock != "75ms" {
				t.Fatalf("unexpected session timeouts: statement=%q lock=%q", statement, lock)
			}
		}
		ctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
		defer cancel()
		if conn, err := pool.Acquire(ctx); err == nil {
			conn.Release()
			t.Fatal("pool exceeded maximum size")
		} else if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("expected acquisition deadline")
		}
	})

	t.Run("statement timeout", func(t *testing.T) {
		_, err := pool.Exec(ctx, "SELECT pg_sleep(5)")
		assertSQLState(t, err, "57014")
		if err := Ping(ctx, pool); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("lock timeout", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal("begin test transaction failed")
		}
		defer tx.Rollback(context.Background())
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(1)"); err != nil {
			t.Fatal("acquire advisory lock failed")
		}
		_, err = pool.Exec(ctx, "SELECT pg_advisory_xact_lock(1)")
		assertSQLState(t, err, "55P03")
	})

	t.Run("authentication errors are safe", func(t *testing.T) {
		parsed, err := url.Parse(dsn)
		if err != nil {
			t.Fatal("parse test connection settings failed")
		}
		parsed.User = url.UserPassword("private_user", "private_incorrect_password")
		badPool, err := NewPool(ctx, parsed.String(), cfg)
		if badPool != nil {
			badPool.Close()
			t.Fatal("expected authentication failure")
		}
		if err == nil || strings.Contains(err.Error(), "private_") || errors.Unwrap(err) != nil {
			t.Fatal("expected safe authentication error")
		}
	})
}

func assertSQLState(t *testing.T, err error, state string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != state {
		t.Fatalf("expected PostgreSQL SQLSTATE %s", state)
	}
}
