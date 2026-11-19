package http

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"log/slog"
	nethttp "net/http"
	"time"

	"github.com/akhmed9505/comment-tree/internal/config"
	"github.com/akhmed9505/comment-tree/internal/domain"
)

type requestIDKey struct{}

// RequestID returns the server-generated identifier attached to a request context.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// NewMiddleware returns middleware for bounded JSON responses, deadlines and request logging.
func NewMiddleware(log *slog.Logger, limits config.Limits) (func(nethttp.Handler) nethttp.Handler, error) {
	if log == nil || limits.RequestTimeout <= 0 || limits.MaxRequestBytes < 1 || limits.MaxConcurrentRequests < 1 {
		return nil, errors.New("HTTP middleware requires a logger and positive request limits")
	}
	// Fallback JSON errors must fit the response limit too.
	if limits.MaxResponseBytes < minimumResponseBytes() {
		return nil, errors.New("HTTP response limit is too small for safe JSON errors")
	}
	slots := make(chan struct{}, limits.MaxConcurrentRequests)
	return func(next nethttp.Handler) nethttp.Handler {
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
				status = responseError(err).status
				fail(w, r, err)
			}
			if err := r.Context().Err(); err != nil {
				respondError(err)
				return
			}
			select {
			case slots <- struct{}{}:
			default:
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
				defer func() {
					if recover() != nil {
						panicked = true
					}
					// Keep the slot until the handler stops, even when the client has already timed out.
					<-slots
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
	}, nil
}

func minimumResponseBytes() int64 {
	var size int64
	for _, err := range []error{domain.ErrInternal, domain.ErrUnavailable, domain.ErrResultTooLarge, context.DeadlineExceeded} {
		e := responseError(err)
		data, _ := json.Marshal(errorResponse{Error: errorDTO{Code: e.code, Message: e.message}})
		size = max(size, int64(len(data)+1))
	}
	return size
}

func protectedHeaders(header nethttp.Header, id string) {
	header.Set("X-Request-ID", id)
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("X-Frame-Options", "DENY")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("Content-Security-Policy", "default-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
	header.Set("Cache-Control", "no-store")
}
