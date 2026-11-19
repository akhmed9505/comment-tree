package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHealthAndReadiness(t *testing.T) {
	calls := 0
	h, err := New(func(ctx context.Context) error {
		calls++
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("readiness deadline missing")
		}
		return nil
	}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.Healthz(w, httptest.NewRequest("GET", "/healthz", nil))
	if w.Code != http.StatusOK || calls != 0 {
		t.Fatal("liveness must not depend on the database")
	}
	w = httptest.NewRecorder()
	h.Readyz(w, httptest.NewRequest("GET", "/readyz", nil))
	if w.Code != http.StatusOK || calls != 1 {
		t.Fatal("readiness must ping")
	}
	h.Stop()
	w = httptest.NewRecorder()
	h.Readyz(w, httptest.NewRequest("GET", "/readyz", nil))
	if w.Code != http.StatusServiceUnavailable || calls != 1 {
		t.Fatal("shutdown must disable readiness")
	}
	h, _ = New(func(context.Context) error { return errors.New("private database credentials") }, time.Second)
	w = httptest.NewRecorder()
	h.Readyz(w, httptest.NewRequest("GET", "/readyz", nil))
	if w.Code != http.StatusServiceUnavailable || strings.Contains(w.Body.String(), "private") {
		t.Fatal("unavailability must be safe")
	}
	h, _ = New(func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }, time.Millisecond)
	w = httptest.NewRecorder()
	h.Readyz(w, httptest.NewRequest("GET", "/readyz", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatal("ping timeout must disable readiness")
	}
	if _, err := New(nil, time.Second); err == nil {
		t.Fatal("nil ping accepted")
	}
	if _, err := New(func(context.Context) error { return nil }, 0); err == nil {
		t.Fatal("zero timeout accepted")
	}
}
