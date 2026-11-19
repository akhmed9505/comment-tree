// Package postgres provides PostgreSQL infrastructure.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/akhmed9505/comment-tree/internal/config"
)

// NewPool opens a pool and verifies connectivity within the configured connect timeout.
// The caller owns the returned pool and must close it.
func NewPool(ctx context.Context, dsn string, cfg config.Postgres) (*pgxpool.Pool, error) {
	poolConfig, err := parseConfig(dsn, cfg)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, errors.New("initialize PostgreSQL pool: invalid pool settings")
	}
	if err := Ping(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

// Ping checks connectivity using the caller's deadline and returns a credential-safe error.
func Ping(ctx context.Context, pool *pgxpool.Pool) error {
	if err := pool.Ping(ctx); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("ping PostgreSQL: %w", ctx.Err())
		}
		return errors.New("ping PostgreSQL: connection unavailable")
	}
	return nil
}

func parseConfig(dsn string, cfg config.Postgres) (*pgxpool.Config, error) {
	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		// pgx parser errors may contain the original DSN, including credentials.
		return nil, errors.New("parse PostgreSQL DSN: invalid connection settings")
	}
	if cfg.MaxConns < 1 || cfg.MaxConns > math.MaxInt32 || cfg.MinConns < 0 || cfg.MinConns > cfg.MaxConns {
		return nil, errors.New("configure PostgreSQL pool: invalid connection limits")
	}
	// PostgreSQL timeout settings use signed 32-bit millisecond values.
	if cfg.ConnectTimeout <= 0 || cfg.StatementTimeout < time.Millisecond || cfg.LockTimeout < time.Millisecond ||
		cfg.StatementTimeout.Milliseconds() > math.MaxInt32 || cfg.LockTimeout.Milliseconds() > math.MaxInt32 {
		return nil, errors.New("configure PostgreSQL pool: timeouts are outside supported ranges")
	}
	// pgx accepts nonpositive health check periods, but the pool's ticker requires a positive interval.
	if poolConfig.HealthCheckPeriod <= 0 || poolConfig.MinIdleConns < 0 || poolConfig.MinIdleConns > int32(cfg.MaxConns) {
		return nil, errors.New("configure PostgreSQL pool: invalid health check or idle connection settings")
	}

	poolConfig.MaxConns = int32(cfg.MaxConns)
	poolConfig.MinConns = int32(cfg.MinConns)
	poolConfig.ConnConfig.ConnectTimeout = cfg.ConnectTimeout
	poolConfig.ConnConfig.RuntimeParams["statement_timeout"] = strconv.FormatInt(cfg.StatementTimeout.Milliseconds(), 10)
	poolConfig.ConnConfig.RuntimeParams["lock_timeout"] = strconv.FormatInt(cfg.LockTimeout.Milliseconds(), 10)
	return poolConfig, nil
}
