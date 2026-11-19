package http

import (
	"context"
	"log/slog"
	"net"
	"net/http"

	"github.com/akhmed9505/comment-tree/internal/config"
)

// NewServer applies configured timeouts and a cancellable connection base context.
func NewServer(cfg config.HTTP, handler http.Handler, ctx context.Context, log *slog.Logger) *http.Server {
	return &http.Server{Addr: cfg.Addr, Handler: handler, ReadHeaderTimeout: cfg.ReadHeaderTimeout, ReadTimeout: cfg.ReadTimeout, WriteTimeout: cfg.WriteTimeout, IdleTimeout: cfg.IdleTimeout, MaxHeaderBytes: cfg.MaxHeaderBytes, BaseContext: func(net.Listener) context.Context { return ctx }, ErrorLog: slog.NewLogLogger(log.Handler(), slog.LevelError)}
}
