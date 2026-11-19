package app

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/akhmed9505/comment-tree/internal/config"
	delivery "github.com/akhmed9505/comment-tree/internal/delivery/http"
	"github.com/akhmed9505/comment-tree/internal/delivery/http/handler/health"
	"github.com/akhmed9505/comment-tree/internal/delivery/http/middleware"
)

func testApplication(t *testing.T, handler http.Handler, timeout time.Duration) (*Application, <-chan struct{}) {
	t.Helper()
	log := slog.New(slog.DiscardHandler)
	controls, err := middleware.New(log, config.Limits{RequestTimeout: 20 * time.Millisecond, MaxRequestBytes: 1024, MaxResponseBytes: 1024, MaxConcurrentRequests: 2})
	if err != nil {
		t.Fatal(err)
	}
	base, cancel := context.WithCancel(context.Background())
	h, _ := health.New(func(context.Context) error { return nil }, time.Second)
	closed := make(chan struct{})
	server := delivery.NewServer(config.HTTP{Addr: "127.0.0.1:0", ReadHeaderTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second, IdleTimeout: time.Second, MaxHeaderBytes: 1024}, controls.Wrap(handler), base, log)
	a := &Application{server: server, middleware: controls, health: h, cancel: cancel, closeResources: func() { close(closed) }, shutdownTimeout: timeout, log: log, stopped: make(chan struct{})}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = a.Shutdown(ctx)
	})
	return a, closed
}

func TestShutdownWaitsForTimedOutHandler(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	a, closed := testApplication(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-release; w.WriteHeader(http.StatusNoContent) }), time.Second)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	runCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- a.Serve(runCtx, listener) }()
	client := &http.Client{Timeout: time.Second}
	res, err := client.Get("http://" + listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusGatewayTimeout {
		t.Fatal("expected HTTP timeout")
	}
	cancel()
	select {
	case <-closed:
		t.Fatal("resources closed while handler was alive")
	case <-time.After(20 * time.Millisecond):
	}
	release <- struct{}{}
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown failed to drain")
	}
	select {
	case <-closed:
	default:
		t.Fatal("resources not released")
	}
	if err := a.Shutdown(t.Context()); err != nil {
		t.Fatal("repeated shutdown must succeed")
	}
}

func TestShutdownDeadlineAndEventualCleanup(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	a, closed := testApplication(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-release }), 20*time.Millisecond)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- a.Serve(ctx, listener) }()
	res, err := (&http.Client{Timeout: time.Second}).Get("http://" + listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected shutdown deadline, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown ignored its deadline")
	}
	select {
	case <-closed:
		t.Fatal("resources closed before background handler stopped")
	default:
	}
	release <- struct{}{}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("eventual cleanup failed")
	}
}

func TestListenFailureReleasesResources(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	a, closed := testApplication(t, http.NotFoundHandler(), time.Second)
	a.server.Addr = occupied.Addr().String()
	if err := a.Run(t.Context()); err == nil {
		t.Fatal("listen failure ignored")
	}
	select {
	case <-closed:
	default:
		t.Fatal("listen failure leaked resources")
	}
}
