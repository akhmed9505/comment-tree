//go:build integration

package integration

import (
	"time"

	"github.com/akhmed9505/comment-tree/internal/config"
)

func testConfig() config.Postgres {
	return config.Postgres{MaxConns: 2, MinConns: 0, ConnectTimeout: 5 * time.Second, StatementTimeout: 1250 * time.Millisecond, LockTimeout: 275 * time.Millisecond}
}
