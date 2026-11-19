package http

import (
	"context"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/akhmed9505/comment-tree/internal/config"
)

func TestServerConfiguration(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	cfg := config.HTTP{Addr: "127.0.0.1:8080", ReadHeaderTimeout: time.Second, ReadTimeout: 2 * time.Second, WriteTimeout: 3 * time.Second, IdleTimeout: 4 * time.Second, MaxHeaderBytes: 8192}
	s := NewServer(cfg, http.NotFoundHandler(), ctx, slog.New(slog.DiscardHandler))
	if s.Addr != cfg.Addr || s.ReadHeaderTimeout != cfg.ReadHeaderTimeout || s.ReadTimeout != cfg.ReadTimeout || s.WriteTimeout != cfg.WriteTimeout || s.IdleTimeout != cfg.IdleTimeout || s.MaxHeaderBytes != cfg.MaxHeaderBytes || s.BaseContext(nil) != ctx || s.ErrorLog == nil {
		t.Fatal("HTTP server configuration was not applied")
	}
}
