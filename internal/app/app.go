// Package app constructs dependencies and coordinates application lifetime.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/akhmed9505/comment-tree/internal/config"
	delivery "github.com/akhmed9505/comment-tree/internal/delivery/http"
	commenthandler "github.com/akhmed9505/comment-tree/internal/delivery/http/handler/comments"
	"github.com/akhmed9505/comment-tree/internal/delivery/http/handler/health"
	"github.com/akhmed9505/comment-tree/internal/delivery/http/middleware"
	"github.com/akhmed9505/comment-tree/internal/infra/postgres"
	"github.com/akhmed9505/comment-tree/internal/logger"
	repository "github.com/akhmed9505/comment-tree/internal/repository/comments"
	service "github.com/akhmed9505/comment-tree/internal/service/comments"
)

// Application owns the HTTP server and its dependency lifecycle.
type Application struct {
	server          *http.Server
	middleware      *middleware.Controller
	health          *health.Handler
	cancel          context.CancelFunc
	closeResources  func()
	shutdownTimeout time.Duration
	log             *slog.Logger
	stopOnce        sync.Once
	stopped         chan struct{}
	shutdownErr     error
}

// New constructs the application and releases the pool if later initialization fails.
// The caller must run or shut down a successfully constructed application.
func New(ctx context.Context, cfg config.Config, log *slog.Logger) (*Application, error) {
	if log == nil || cfg.HTTP.ShutdownTimeout <= 0 {
		return nil, errors.New("application requires a logger and positive shutdown timeout")
	}
	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL, cfg.Postgres)
	if err != nil {
		return nil, err
	}
	initialized := false
	defer func() {
		if !initialized {
			pool.Close()
		}
	}()
	svc, err := service.New(repository.New(pool), cfg.Limits.MaxResultNodes)
	if err != nil {
		return nil, err
	}
	comments, err := commenthandler.New(svc)
	if err != nil {
		return nil, err
	}
	healthHandler, err := health.New(func(ctx context.Context) error { return postgres.Ping(ctx, pool) }, cfg.Postgres.ConnectTimeout)
	if err != nil {
		return nil, err
	}
	controls, err := middleware.New(log, cfg.Limits)
	if err != nil {
		return nil, err
	}
	base, cancel := context.WithCancel(context.Background())
	server := delivery.NewServer(cfg.HTTP, controls.Wrap(delivery.NewRouter(comments, healthHandler)), base, log)
	initialized = true
	return &Application{server: server, middleware: controls, health: healthHandler, cancel: cancel, closeResources: pool.Close, shutdownTimeout: cfg.HTTP.ShutdownTimeout, log: log, stopped: make(chan struct{})}, nil
}

// Run loads configuration, installs signal handling and runs the application.
func Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	log, err := logger.New(cfg.Logging, os.Stdout)
	if err != nil {
		return err
	}
	a, err := New(ctx, cfg, log)
	if err != nil {
		return fmt.Errorf("initialize application: %w", err)
	}
	return a.Run(ctx)
}

// Run listens at the configured address until cancellation or a server failure.
func (a *Application) Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", a.server.Addr)
	if err != nil {
		return errors.Join(fmt.Errorf("listen HTTP: %w", err), a.stop())
	}
	return a.Serve(ctx, listener)
}

// Serve owns listener and drains the application when serving stops.
func (a *Application) Serve(ctx context.Context, listener net.Listener) error {
	result := make(chan error, 1)
	go func() { result <- a.server.Serve(listener) }()
	a.log.Info("HTTP server started", "addr", listener.Addr().String())
	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-result:
	}
	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}
	err := a.stop()
	a.log.Info("HTTP server stopped")
	return errors.Join(serveErr, err)
}

func (a *Application) stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), a.shutdownTimeout)
	defer cancel()
	return a.Shutdown(ctx)
}

// Shutdown bounds caller waiting while cleanup continues until background handlers release resources.
func (a *Application) Shutdown(ctx context.Context) error {
	a.stopOnce.Do(func() {
		a.health.Stop()
		go func() {
			defer close(a.stopped)
			force := context.AfterFunc(ctx, func() { a.cancel(); _ = a.server.Close() })
			defer force()
			a.shutdownErr = a.server.Shutdown(ctx)
			a.cancel()
			if a.shutdownErr != nil {
				_ = a.server.Close()
			}
			// Closing the pool before these handlers finish could race their database operations.
			_ = a.middleware.Shutdown(context.Background())
			a.closeResources()
		}()
	})
	select {
	case <-a.stopped:
		return a.shutdownErr
	case <-ctx.Done():
		return ctx.Err()
	}
}
