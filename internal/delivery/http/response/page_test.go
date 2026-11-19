package response_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/akhmed9505/comment-tree/internal/config"
	"github.com/akhmed9505/comment-tree/internal/delivery/http/middleware"
	"github.com/akhmed9505/comment-tree/internal/delivery/http/response"
)

type limitedRecorder struct {
	*httptest.ResponseRecorder
	limit   int64
	headers int
}

func (w *limitedRecorder) ResponseLimit() int64 { return w.limit }

func (w *limitedRecorder) WriteHeader(status int) {
	w.headers++
	w.ResponseRecorder.WriteHeader(status)
}

func TestPageByteBoundaries(t *testing.T) {
	escaped := "Русский English <>&\"\\\n\t\x00\u2028\u2029"
	for _, field := range []string{"comments", "matches", "comments<&\""} {
		for _, cursor := range []*string{nil, &escaped} {
			items := []string{strings.Repeat(escaped, 5), "second item"}
			want, err := json.Marshal(map[string]any{field: items, "next_cursor": cursor})
			if err != nil {
				t.Fatal(err)
			}
			want = append(want, '\n')
			for _, extra := range []int{-1, 0, 1} {
				w := &limitedRecorder{ResponseRecorder: httptest.NewRecorder(), limit: int64(len(want) + extra)}
				response.WritePage(w, httptest.NewRequest(http.MethodGet, "/", nil), field, items, func(s string) string { return s }, cursor)
				if w.headers != 1 || !json.Valid(w.Body.Bytes()) {
					t.Fatal("page must commit exactly one complete JSON response")
				}
				if extra < 0 {
					checkError(t, w.ResponseRecorder, http.StatusRequestEntityTooLarge, "result_too_large")
				} else if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), want) {
					t.Fatal("page changed its JSON representation or rejected the exact byte boundary")
				}
			}
		}
	}
}

func TestPageEnvelopeAndCursorBudget(t *testing.T) {
	cursor := strings.Repeat("<&", 20)
	for _, next := range []*string{nil, &cursor} {
		want, _ := json.Marshal(map[string]any{"comments": []string{}, "next_cursor": next})
		want = append(want, '\n')
		for limit := 0; limit <= len(want); limit++ {
			w := &limitedRecorder{ResponseRecorder: httptest.NewRecorder(), limit: int64(limit)}
			response.WritePage(w, httptest.NewRequest(http.MethodGet, "/", nil), "comments", []string(nil), func(s string) string { return s }, next)
			if limit < len(want) {
				checkError(t, w.ResponseRecorder, http.StatusRequestEntityTooLarge, "result_too_large")
			} else if !bytes.Equal(w.Body.Bytes(), want) {
				t.Fatal("empty page must contain an array and its complete cursor")
			}
		}
	}
}

func TestPageStopsBeforeEncodingRemainingItems(t *testing.T) {
	items := make([]string, 10000)
	body := strings.Repeat("a", 10000)
	for i := range items {
		items[i] = body
	}
	encoded := 0
	w := &limitedRecorder{ResponseRecorder: httptest.NewRecorder(), limit: 32768}
	response.WritePage(w, httptest.NewRequest(http.MethodGet, "/", nil), "comments", items, func(s string) string {
		encoded++
		return s
	}, nil)
	checkError(t, w.ResponseRecorder, http.StatusRequestEntityTooLarge, "result_too_large")
	if encoded != 4 || w.headers != 1 || int64(w.Body.Len()) > w.limit {
		t.Fatal("encoder must stop at the first overflowing item without committing a partial page")
	}
}

func TestPageEncodingFailureDiscardsEarlierItems(t *testing.T) {
	w := &limitedRecorder{ResponseRecorder: httptest.NewRecorder(), limit: 4096}
	response.WritePage(w, httptest.NewRequest(http.MethodGet, "/", nil), "matches", []any{"already encoded", make(chan int)}, func(v any) any { return v }, nil)
	checkError(t, w.ResponseRecorder, http.StatusInternalServerError, "internal_error")
	if w.headers != 1 || strings.Contains(w.Body.String(), "already encoded") {
		t.Fatal("encoding failure exposed partial JSON")
	}
}

func TestPageCancellationDuringEncoding(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	w := &limitedRecorder{ResponseRecorder: httptest.NewRecorder(), limit: 4096}
	encoded := 0
	response.WritePage(w, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx), "comments", []int{1, 2, 3}, func(v int) int {
		encoded++
		cancel()
		return v
	}, nil)
	if encoded != 1 || w.headers != 0 || w.Body.Len() != 0 {
		t.Fatal("canceled encoding must stop without sending headers or a partial body")
	}
}

func TestPageWithMiddleware(t *testing.T) {
	for _, scenario := range []string{"overflow", "timeout", "cancel"} {
		t.Run(scenario, func(t *testing.T) {
			limits := config.Limits{RequestTimeout: 20 * time.Millisecond, MaxRequestBytes: 1024, MaxResponseBytes: 1024, MaxConcurrentRequests: 1}
			controller, err := middleware.New(slog.New(slog.DiscardHandler), limits)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			handler := controller.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				response.WritePage(w, r, "comments", []string{"first", strings.Repeat("second", 1000), "last"}, func(s string) string {
					if scenario == "timeout" {
						<-r.Context().Done()
					}
					if scenario == "cancel" {
						cancel()
					}
					return s
				}, nil)
			}))
			w := &limitedRecorder{ResponseRecorder: httptest.NewRecorder()}
			handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx))
			shutdown, stop := context.WithTimeout(t.Context(), time.Second)
			defer stop()
			if err := controller.Shutdown(shutdown); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "overflow":
				checkError(t, w.ResponseRecorder, http.StatusRequestEntityTooLarge, "result_too_large")
			case "timeout":
				checkError(t, w.ResponseRecorder, http.StatusGatewayTimeout, "deadline_exceeded")
			case "cancel":
				if w.headers != 0 || w.Body.Len() != 0 {
					t.Fatal("canceled request sent a response")
				}
			}
			if scenario != "cancel" && w.headers != 1 {
				t.Fatal("response headers were committed more than once")
			}
		})
	}
}

func checkError(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	var got response.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if w.Code != status || got.Error.Code != code || !json.Valid(w.Body.Bytes()) {
		t.Fatalf("expected complete %d %s error, got %d %s", status, code, w.Code, w.Body.String())
	}
}
