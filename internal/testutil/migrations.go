//go:build integration

package testutil

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Migrate runs Goose CLI against the project's SQL migrations without exposing its diagnostics.
func Migrate(t testing.TB, dsn, command string) {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatal("locate project directory failed")
	}
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Fatal("locate project go.mod failed")
		}
		root = parent
	}
	if _, err := exec.LookPath("goose"); err != nil {
		t.Fatal("Goose CLI is required in PATH for migration tests")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "goose", "-env", "none", "-dir", filepath.Join(root, "migrations"), "-timeout", "10s", command)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GOOSE_") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "GOOSE_DRIVER=postgres", "GOOSE_DBSTRING="+dsn)
	if err := cmd.Run(); err != nil {
		t.Fatalf("goose %s failed; diagnostics omitted to protect connection credentials", command)
	}
}
