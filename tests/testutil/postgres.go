//go:build integration

// Package testutil provides integration test infrastructure.
package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// StartPostgres returns a DSN for an isolated PostgreSQL container removed at test cleanup.
func StartPostgres(t testing.TB) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	container, err := postgres.Run(ctx, "postgres:18.6-alpine",
		postgres.WithDatabase("commenttree_test"),
		postgres.WithUsername("commenttree_test"),
		postgres.WithPassword("commenttree_test_password"),
		postgres.BasicWaitStrategies(),
	)
	testcontainers.CleanupContainer(t, container)
	if err != nil {
		t.Fatal("start PostgreSQL test container: check Docker availability and image access")
	}
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal("get PostgreSQL test connection settings: container endpoint unavailable")
	}
	return dsn
}
