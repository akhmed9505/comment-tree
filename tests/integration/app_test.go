//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/akhmed9505/comment-tree/internal/app"
	"github.com/akhmed9505/comment-tree/internal/config"
	"github.com/akhmed9505/comment-tree/internal/infra/postgres"
	"github.com/akhmed9505/comment-tree/tests/testutil"
)

func TestApplicationWithPostgres(t *testing.T) {
	dsn := testutil.StartPostgres(t)
	testutil.Migrate(t, dsn, "up")
	cfg := config.Config{DatabaseURL: dsn, Postgres: testConfig(), HTTP: config.HTTP{Addr: "127.0.0.1:8080", ReadHeaderTimeout: time.Second, ReadTimeout: 2 * time.Second, WriteTimeout: 3 * time.Second, IdleTimeout: time.Second, ShutdownTimeout: time.Second, MaxHeaderBytes: 16384}, Limits: config.Limits{RequestTimeout: time.Second, MaxRequestBytes: 65536, MaxResponseBytes: 1 << 20, MaxResultNodes: 100, MaxConcurrentRequests: 4}}
	log := slog.New(slog.DiscardHandler)
	a, err := app.New(t.Context(), cfg, log)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = a.Shutdown(ctx)
	}()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- a.Serve(ctx, listener) }()
	client := &http.Client{Timeout: 2 * time.Second}
	base := "http://" + listener.Addr().String()
	for _, path := range []string{"/healthz", "/readyz", "/comments"} {
		res, err := client.Get(base + path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusOK || res.Header.Get("X-Request-ID") == "" {
			t.Fatal("live application endpoint failed")
		}
	}
	res, err := client.Post(base+"/comments", "application/json", strings.NewReader(`{"author":"Author","body":"Кошки running"}`))
	if err != nil {
		t.Fatal(err)
	}
	var comment struct {
		ID string `json:"id"`
	}
	if json.NewDecoder(res.Body).Decode(&comment) != nil || res.StatusCode != http.StatusCreated {
		t.Fatal("live create failed")
	}
	res.Body.Close()
	for _, path := range []string{"/comments?parent=" + comment.ID, "/comments/search?q=кошка"} {
		res, err := client.Get(base + path)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != http.StatusOK || !strings.Contains(string(data), comment.ID) {
			t.Fatal("live read/search failed")
		}
	}
	req, _ := http.NewRequest(http.MethodDelete, base+"/comments/"+comment.ID, nil)
	res, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatal("live deletion failed")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("application did not stop")
	}
	// A failed constructor must close the already initialized pool before returning.
	bad := cfg
	bad.Limits.MaxResponseBytes = 1
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal("parse fixture DSN failed")
	}
	values := parsed.Query()
	values.Set("application_name", "commenttree_init_failure")
	parsed.RawQuery = values.Encode()
	bad.DatabaseURL = parsed.String()
	if _, err := app.New(t.Context(), bad, log); err == nil {
		t.Fatal("invalid middleware configuration accepted")
	}
	observer, err := postgres.NewPool(t.Context(), dsn, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close()
	var count int
	if err := observer.QueryRow(t.Context(), "SELECT count(*) FROM pg_stat_activity WHERE application_name='commenttree_init_failure'").Scan(&count); err != nil || count != 0 {
		t.Fatal("failed initialization leaked database connections")
	}
}
