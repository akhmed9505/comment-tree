// Package health provides liveness and database readiness checks.
package health

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/akhmed9505/comment-tree/internal/delivery/http/response"
	"github.com/akhmed9505/comment-tree/internal/domain"
)

// Handler checks readiness without coupling HTTP to a database driver.
type Handler struct {
	ping     func(context.Context) error
	timeout  time.Duration
	stopping atomic.Bool
}

// New accepts a bounded connectivity check for readiness.
func New(ping func(context.Context) error, timeout time.Duration) (*Handler, error) {
	if ping == nil || timeout <= 0 {
		return nil, errors.New("readiness requires a ping function and positive timeout")
	}
	return &Handler{ping: ping, timeout: timeout}, nil
}

// Stop marks the application unready before shutdown starts.
func (h *Handler) Stop() { h.stopping.Store(true) }

// Healthz reports that the process can serve HTTP requests.
func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	response.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Readyz reports database availability unless shutdown has started.
func (h *Handler) Readyz(w http.ResponseWriter, r *http.Request) {
	if h.stopping.Load() {
		response.Fail(w, r, domain.ErrUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	if err := h.ping(ctx); err != nil || h.stopping.Load() {
		response.Fail(w, r, domain.ErrUnavailable)
		return
	}
	response.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
