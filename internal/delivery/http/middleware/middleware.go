package middleware

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	nethttp "net/http"
	"sync"
	"time"

	"github.com/akhmed9505/comment-tree/internal/config"
	"github.com/akhmed9505/comment-tree/internal/delivery/http/response"
	"github.com/akhmed9505/comment-tree/internal/domain"
)

type requestIDKey struct{}

// RequestID returns the server-generated identifier attached to a request context.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// Controller tracks handlers independently of HTTP response deadlines.
type Controller struct {
	wrap          func(nethttp.Handler) nethttp.Handler
	mu            sync.Mutex
	active, limit int
	draining      bool
	idle          chan struct{}
}

// New builds middleware with lifecycle tracking for background handlers.
func New(log *slog.Logger, limits config.Limits) (*Controller, error) {
	if log == nil || limits.RequestTimeout <= 0 || limits.MaxRequestBytes < 1 || limits.MaxConcurrentRequests < 1 {
		return nil, errors.New("HTTP middleware requires a logger and positive request limits")
	}
	// Fallback JSON errors must fit the response limit too.
	if limits.MaxResponseBytes < response.MinimumErrorBytes() {
		return nil, errors.New("HTTP response limit is too small for safe JSON errors")
	}
	c := &Controller{limit: limits.MaxConcurrentRequests, idle: make(chan struct{})}
	close(c.idle)
	c.wrap = func(next nethttp.Handler) nethttp.Handler {
		return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
			start := time.Now()
			method, path := r.Method, r.URL.Path
			id := rand.Text()
			r = r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id))
			protectedHeaders(w.Header(), id)
			responseDone := make(chan struct{})
			status := 0
			recovered := false
			defer func() {
				defer close(responseDone)
				level := slog.LevelInfo
				if recovered {
					level = slog.LevelError
				}
				log.Log(r.Context(), level, "HTTP request", "request_id", id, "method", method, "path", path, "status", status, "duration", time.Since(start), "panic_recovered", recovered)
			}()
			respondError := func(err error) {
				if errors.Is(r.Context().Err(), context.Canceled) {
					return
				}
				status = response.ErrorFor(err).Status
				response.Fail(w, r, err)
			}
			if err := r.Context().Err(); err != nil {
				respondError(err)
				return
			}
			if !c.enter() {
				respondError(domain.ErrUnavailable)
				return
			}
			timeoutCtx, stop := context.WithTimeout(r.Context(), limits.RequestTimeout)
			defer stop()
			ctx, cancel := context.WithCancelCause(timeoutCtx)
			defer cancel(nil)
			buffered := &boundedResponse{header: make(nethttp.Header), limit: limits.MaxResponseBytes, ctx: ctx, cancel: cancel}
			rWithTimeout := r.WithContext(ctx)
			rWithTimeout.Body = nethttp.MaxBytesReader(buffered, r.Body, limits.MaxRequestBytes)
			done := make(chan struct{})
			panicked := false
			go func() {
				defer c.leave()
				defer func() {
					if recover() != nil {
						panicked = true
					}
					close(done)
					if panicked {
						// The response decision is final before deciding whether a separate panic event is needed.
						<-responseDone
						if !recovered {
							log.ErrorContext(context.WithoutCancel(r.Context()), "HTTP handler panic after response", "request_id", id, "method", method, "path", path, "duration", time.Since(start), "panic_recovered", true)
						}
					}
				}()
				next.ServeHTTP(buffered, rWithTimeout)
			}()
			select {
			case <-ctx.Done():
				respondError(context.Cause(ctx))
			case <-done:
				// The buffer belongs to the handler goroutine until done closes.
				if ctx.Err() != nil {
					respondError(context.Cause(ctx))
					return
				}
				if panicked {
					recovered = true
					respondError(domain.ErrInternal)
					return
				}
				for key, values := range buffered.header {
					w.Header()[key] = append([]string(nil), values...)
				}
				protectedHeaders(w.Header(), id)
				status = buffered.status
				if status == 0 {
					status = nethttp.StatusOK
				}
				w.WriteHeader(status)
				if len(buffered.body) > 0 {
					_, _ = w.Write(buffered.body)
				}
			}
		})
	}
	return c, nil
}

func protectedHeaders(header nethttp.Header, id string) {
	header.Set("X-Request-ID", id)
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("X-Frame-Options", "DENY")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("Content-Security-Policy", "default-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
	header.Set("Cache-Control", "no-store")
}

// Wrap applies the middleware to a finite HTTP handler.
func (c *Controller) Wrap(next nethttp.Handler) nethttp.Handler { return c.wrap(next) }

// Shutdown stops admission and waits for all handlers, including those past their HTTP deadline.
func (c *Controller) Shutdown(ctx context.Context) error {
	c.mu.Lock()
	c.draining = true
	idle := c.idle
	c.mu.Unlock()
	select {
	case <-idle:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Controller) enter() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.draining || c.active >= c.limit {
		return false
	}
	if c.active == 0 {
		c.idle = make(chan struct{})
	}
	c.active++
	return true
}

func (c *Controller) leave() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.active--
	if c.active == 0 {
		close(c.idle)
	}
}
