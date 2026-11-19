package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/akhmed9505/comment-tree/internal/config"
	"github.com/akhmed9505/comment-tree/internal/delivery/http/response"
	"github.com/akhmed9505/comment-tree/internal/domain"
	"github.com/akhmed9505/comment-tree/internal/logger"
)

func middlewareLimits() config.Limits {
	return config.Limits{RequestTimeout: time.Second, MaxRequestBytes: 64, MaxResponseBytes: 1024, MaxConcurrentRequests: 2}
}

func wrapTest(t *testing.T, limits config.Limits, output io.Writer, next nethttp.Handler) nethttp.Handler {
	t.Helper()
	log, err := logger.New(config.Logging{Level: "debug", Format: "json"}, output)
	if err != nil {
		t.Fatal(err)
	}
	wrap, err := New(log, limits)
	if err != nil {
		t.Fatal(err)
	}
	return wrap.Wrap(next)
}

type countingWriter struct {
	*httptest.ResponseRecorder
	headers int
}

func (w *countingWriter) WriteHeader(status int) { w.headers++; w.ResponseRecorder.WriteHeader(status) }

func TestMiddlewareMetadataAndLogging(t *testing.T) {
	var logs bytes.Buffer
	var ids []string
	h := wrapTest(t, middlewareLimits(), &logs, nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		id := RequestID(r.Context())
		if id == "" {
			t.Error("request ID missing from context")
		}
		ids = append(ids, id)
		w.Header().Set("X-Request-ID", "forged")
		w.Header().Set("X-Content-Type-Options", "forged")
		response.WriteJSON(w, nethttp.StatusCreated, map[string]string{"value": "ok"})
		w.WriteHeader(nethttp.StatusInternalServerError)
	}))
	for range 2 {
		r := httptest.NewRequest("POST", "/comments?private_query=secret", nil)
		r.Header.Set("X-Request-ID", "untrusted-secret")
		w := &countingWriter{ResponseRecorder: httptest.NewRecorder()}
		h.ServeHTTP(w, r)
		if w.Code != nethttp.StatusCreated || w.headers != 1 || w.Header().Get("X-Request-ID") != ids[len(ids)-1] {
			t.Fatal("response status or request ID changed")
		}
		for key, want := range map[string]string{"X-Content-Type-Options": "nosniff", "X-Frame-Options": "DENY", "Referrer-Policy": "no-referrer", "Cache-Control": "no-store"} {
			if w.Header().Get(key) != want {
				t.Fatalf("missing protective header %s", key)
			}
		}
		if w.Header().Get("Content-Security-Policy") == "" {
			t.Fatal("missing CSP")
		}
	}
	if ids[0] == ids[1] || RequestID(t.Context()) != "" {
		t.Fatal("request ID scope is incorrect")
	}
	if strings.Contains(logs.String(), "secret") || strings.Contains(logs.String(), "private_query") {
		t.Fatal("query string or untrusted request ID leaked")
	}
	dec := json.NewDecoder(&logs)
	for i := range 2 {
		var event map[string]any
		if err := dec.Decode(&event); err != nil {
			t.Fatal(err)
		}
		if event["request_id"] != ids[i] || event["method"] != "POST" || event["path"] != "/comments" || event["status"] != float64(nethttp.StatusCreated) || event["duration"] == nil {
			t.Fatal("request log fields missing or incorrect")
		}
	}
}

func TestMiddlewareResponseBoundaries(t *testing.T) {
	limit := response.MinimumErrorBytes() + 20
	for _, extra := range []int{-1, 0, 1} {
		t.Run(fmt.Sprint(extra), func(t *testing.T) {
			limits := middlewareLimits()
			limits.MaxResponseBytes = limit
			h := wrapTest(t, limits, io.Discard, nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
				w.Header().Set("X-Partial", "private")
				response.WriteJSON(w, nethttp.StatusCreated, strings.Repeat("x", int(limit)-3+extra))
			}))
			w := &countingWriter{ResponseRecorder: httptest.NewRecorder()}
			h.ServeHTTP(w, httptest.NewRequest("GET", "/comments", nil))
			if w.headers != 1 {
				t.Fatal("response headers sent more than once")
			}
			if extra <= 0 {
				if w.Code != nethttp.StatusCreated || int64(w.Body.Len()) != limit+int64(extra) {
					t.Fatal("valid response boundary rejected")
				}
			} else {
				if w.Code != nethttp.StatusRequestEntityTooLarge || w.Header().Get("X-Partial") != "" || int64(w.Body.Len()) > limit {
					t.Fatal("overflow exposed partial response or exceeded limit")
				}
				var body response.ErrorResponse
				if json.Unmarshal(w.Body.Bytes(), &body) != nil || body.Error.Code != "result_too_large" {
					t.Fatal("overflow must use the standard error format")
				}
			}
		})
	}
	t.Run("multiple writes", func(t *testing.T) {
		limits := middlewareLimits()
		limits.MaxResponseBytes = limit
		h := wrapTest(t, limits, io.Discard, nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
			_, _ = w.Write([]byte(strings.Repeat("x", int(limit))))
			if _, err := w.Write([]byte("x")); !errors.Is(err, domain.ErrResultTooLarge) {
				t.Error("writer must return the domain limit error")
			}
		}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		if w.Code != nethttp.StatusRequestEntityTooLarge || strings.Contains(w.Body.String(), "xxx") {
			t.Fatal("partial writes escaped buffer")
		}
	})
}

func TestMiddlewarePanicRecovery(t *testing.T) {
	for _, writeFirst := range []bool{false, true} {
		t.Run(fmt.Sprint(writeFirst), func(t *testing.T) {
			var logs bytes.Buffer
			h := wrapTest(t, middlewareLimits(), &logs, nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
				if writeFirst {
					w.Header().Set("X-Partial", "private")
					w.WriteHeader(nethttp.StatusCreated)
					_, _ = w.Write([]byte("private partial body"))
				}
				panic("private panic diagnostic")
			}))
			w := &countingWriter{ResponseRecorder: httptest.NewRecorder()}
			h.ServeHTTP(w, httptest.NewRequest("GET", "/comments?q=private", nil))
			if w.headers != 1 || w.Code != nethttp.StatusInternalServerError || strings.Contains(w.Body.String(), "private") || w.Header().Get("X-Partial") != "" {
				t.Fatal("panic exposed a partial response")
			}
			var body response.ErrorResponse
			if json.Unmarshal(w.Body.Bytes(), &body) != nil || body.Error.Code != "internal_error" {
				t.Fatal("panic must use JSON errors")
			}
			var event map[string]any
			if json.Unmarshal(logs.Bytes(), &event) != nil || event["panic_recovered"] != true || event["status"] != float64(nethttp.StatusInternalServerError) || strings.Contains(logs.String(), "private") {
				t.Fatal("panic log exposed diagnostics or lost metadata")
			}
		})
	}
}

func TestMiddlewareTimeoutAndLateWrites(t *testing.T) {
	limits := middlewareLimits()
	limits.RequestTimeout = 20 * time.Millisecond
	limits.MaxConcurrentRequests = 1
	release := make(chan struct{})
	defer close(release)
	observed := make(chan context.Context, 1)
	late := make(chan error, 1)
	h := wrapTest(t, limits, io.Discard, nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		observed <- r.Context()
		<-release
		w.Header().Set("X-Late", "private")
		w.WriteHeader(nethttp.StatusCreated)
		_, err := w.Write([]byte("private late body"))
		late <- err
	}))
	w := &countingWriter{ResponseRecorder: httptest.NewRecorder()}
	done := make(chan struct{})
	go func() { h.ServeHTTP(w, httptest.NewRequest("GET", "/comments", nil)); close(done) }()
	ctx := <-observed
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > limits.RequestTimeout {
		t.Fatal("configured deadline not propagated")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout did not terminate the HTTP response")
	}
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) || w.Code != nethttp.StatusGatewayTimeout || w.headers != 1 {
		t.Fatal("deadline was not propagated or safely reported")
	}
	before := w.Body.String()
	second := httptest.NewRecorder()
	h.ServeHTTP(second, httptest.NewRequest("GET", "/comments", nil))
	if second.Code != nethttp.StatusServiceUnavailable {
		t.Fatal("timed out handler must retain its concurrency slot until it exits")
	}
	release <- struct{}{}
	select {
	case err := <-late:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("late write must fail")
		}
	case <-time.After(time.Second):
		t.Fatal("late handler did not stop")
	}
	if w.Body.String() != before || w.Header().Get("X-Late") != "" || w.headers != 1 {
		t.Fatal("late handler changed the completed response")
	}
}

func TestMiddlewareConcurrencyAndCancellation(t *testing.T) {
	limits := middlewareLimits()
	limits.MaxConcurrentRequests = 1
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	defer close(release)
	h := wrapTest(t, limits, io.Discard, nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		entered <- struct{}{}
		<-release
		w.WriteHeader(nethttp.StatusNoContent)
	}))
	first := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { h.ServeHTTP(first, httptest.NewRequest("GET", "/", nil)); close(done) }()
	<-entered
	second := httptest.NewRecorder()
	h.ServeHTTP(second, httptest.NewRequest("GET", "/", nil))
	if second.Code != nethttp.StatusServiceUnavailable {
		t.Fatal("concurrency limit not enforced")
	}
	release <- struct{}{}
	<-done
	if first.Code != nethttp.StatusNoContent || first.Body.Len() != 0 {
		t.Fatal("normal response changed")
	}
	third := httptest.NewRecorder()
	thirdDone := make(chan struct{})
	go func() { h.ServeHTTP(third, httptest.NewRequest("GET", "/", nil)); close(thirdDone) }()
	<-entered
	release <- struct{}{}
	<-thirdDone
	if third.Code != nethttp.StatusNoContent {
		t.Fatal("finished handler did not release its concurrency slot")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	w := &countingWriter{ResponseRecorder: httptest.NewRecorder()}
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil).WithContext(ctx))
	if w.headers != 0 || w.Body.Len() != 0 {
		t.Fatal("client cancellation must not send a response")
	}
	h2 := wrapTest(t, limits, io.Discard, nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) { w.WriteHeader(nethttp.StatusNoContent) }))
	w2 := httptest.NewRecorder()
	h2.ServeHTTP(w2, httptest.NewRequest("GET", "/", nil))
	if w2.Code != nethttp.StatusNoContent {
		t.Fatal("empty response changed")
	}
}

func TestMiddlewareConfiguration(t *testing.T) {
	log := slog.New(slog.DiscardHandler)
	if _, err := New(nil, middlewareLimits()); err == nil {
		t.Fatal("nil logger accepted")
	}
	for _, change := range []func(*config.Limits){
		func(l *config.Limits) { l.RequestTimeout = 0 }, func(l *config.Limits) { l.MaxRequestBytes = 0 }, func(l *config.Limits) { l.MaxConcurrentRequests = 0 }, func(l *config.Limits) { l.MaxResponseBytes = response.MinimumErrorBytes() - 1 },
	} {
		limits := middlewareLimits()
		change(&limits)
		if _, err := New(log, limits); err == nil {
			t.Fatal("invalid middleware limits accepted")
		}
	}
	limits := middlewareLimits()
	limits.MaxResponseBytes = response.MinimumErrorBytes()
	h := wrapTest(t, limits, io.Discard, nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", int(limits.MaxResponseBytes)+1)))
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != nethttp.StatusRequestEntityTooLarge || int64(w.Body.Len()) > limits.MaxResponseBytes {
		t.Fatal("minimum valid limit must fit fallback JSON")
	}
}

type logEvents chan []byte

func (events logEvents) Write(data []byte) (int, error) {
	events <- append([]byte(nil), data...)
	return len(data), nil
}

func TestMiddlewarePanicLoggingAfterTimeout(t *testing.T) {
	events := make(logEvents, 4)
	limits := middlewareLimits()
	limits.RequestTimeout = 20 * time.Millisecond
	release := make(chan struct{})
	defer close(release)
	h := wrapTest(t, limits, events, nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		<-release
		panic("secret panic diagnostic")
	}))
	w := &countingWriter{ResponseRecorder: httptest.NewRecorder()}
	h.ServeHTTP(w, httptest.NewRequest("GET", "/comments?secret=query", nil))
	if w.Code != nethttp.StatusGatewayTimeout || w.headers != 1 {
		t.Fatal("request must time out before the panic")
	}
	response := w.Body.String()
	release <- struct{}{}
	for i := range 2 {
		var data []byte
		select {
		case data = <-events:
		case <-time.After(time.Second):
			t.Fatal("missing request or late panic log")
		}
		var event map[string]any
		if json.Unmarshal(data, &event) != nil {
			t.Fatal("invalid log event")
		}
		if strings.Contains(string(data), "secret") || event["request_id"] != w.Header().Get("X-Request-ID") || event["path"] != "/comments" || event["method"] != "GET" {
			t.Fatal("late panic log lost correlation or exposed diagnostics")
		}
		if i == 0 && (event["msg"] != "HTTP request" || event["status"] != float64(nethttp.StatusGatewayTimeout)) {
			t.Fatal("request log must retain the timeout status")
		}
		if i == 1 && (event["level"] != "ERROR" || event["panic_recovered"] != true || event["msg"] != "HTTP handler panic after response") {
			t.Fatal("late panic must be logged safely at error level")
		}
	}
	if w.Body.String() != response || w.headers != 1 {
		t.Fatal("late panic changed the completed response")
	}
	select {
	case <-events:
		t.Fatal("duplicate panic log")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestMiddlewareOrdinaryPanicIsLoggedOnce(t *testing.T) {
	events := make(logEvents, 4)
	h := wrapTest(t, middlewareLimits(), events, nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) { panic("secret") }))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/comments", nil))
	select {
	case data := <-events:
		var event map[string]any
		if json.Unmarshal(data, &event) != nil || event["msg"] != "HTTP request" || event["panic_recovered"] != true || event["status"] != float64(nethttp.StatusInternalServerError) || strings.Contains(string(data), "secret") {
			t.Fatal("ordinary panic log is incorrect or unsafe")
		}
	case <-time.After(time.Second):
		t.Fatal("missing panic log")
	}
	select {
	case <-events:
		t.Fatal("ordinary panic was logged twice")
	case <-time.After(20 * time.Millisecond):
	}
}
