package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/akhmed9505/comment-tree/internal/domain"
)

type contextKey struct{}

type serviceStub struct {
	create  func(context.Context, domain.CreateCommentInput) (domain.Comment, error)
	delete  func(context.Context, int64) error
	subtree func(context.Context, int64) ([]domain.Comment, error)
	roots   func(context.Context, domain.RootPageOptions) (domain.RootPage, error)
	search  func(context.Context, domain.SearchOptions) (domain.SearchPage, error)
}

func (s serviceStub) Create(c context.Context, i domain.CreateCommentInput) (domain.Comment, error) {
	return s.create(c, i)
}

func (s serviceStub) Delete(c context.Context, id int64) error { return s.delete(c, id) }

func (s serviceStub) GetSubtree(c context.Context, id int64) ([]domain.Comment, error) {
	return s.subtree(c, id)
}

func (s serviceStub) ListRoots(c context.Context, o domain.RootPageOptions) (domain.RootPage, error) {
	return s.roots(c, o)
}

func (s serviceStub) Search(c context.Context, o domain.SearchOptions) (domain.SearchPage, error) {
	return s.search(c, o)
}

func request(t *testing.T, s Service, method, path, body string, maxBytes int64) *httptest.ResponseRecorder {
	t.Helper()
	h, err := New(s)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(method, path, strings.NewReader(body)).WithContext(context.WithValue(t.Context(), contextKey{}, t.Context()))
	r.Header.Set("Content-Type", "application/json")
	limits := middlewareLimits()
	limits.MaxRequestBytes = maxBytes
	h = wrapTest(t, limits, io.Discard, h)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestSuccess(t *testing.T) {
	id := int64(9223372036854775806)
	parent := int64(1)
	stamp := time.Date(2025, 1, 2, 3, 4, 5, 123456789, time.UTC)
	c := domain.Comment{ID: id, ParentID: &parent, Author: "Автор", Body: "Body", CreatedAt: stamp}
	check := func(ctx context.Context) {
		t.Helper()
		if ctx.Value(contextKey{}) != t.Context() {
			t.Fatal("request context replaced")
		}
	}
	s := serviceStub{
		create: func(ctx context.Context, in domain.CreateCommentInput) (domain.Comment, error) {
			check(ctx)
			if in.ParentID == nil || *in.ParentID != 1 || in.Author != " Автор " {
				t.Fatal("invalid create input")
			}
			return c, nil
		},
		delete: func(ctx context.Context, got int64) error {
			check(ctx)
			if got != id {
				t.Fatal("ID precision lost")
			}
			return nil
		},
		subtree: func(ctx context.Context, got int64) ([]domain.Comment, error) {
			check(ctx)
			if got != id {
				t.Fatal("wrong subtree ID")
			}
			return []domain.Comment{c}, nil
		},
		roots: func(ctx context.Context, o domain.RootPageOptions) (domain.RootPage, error) {
			check(ctx)
			if o.Order != domain.RootASC || o.Limit != 2 {
				t.Fatal("root parameters lost")
			}
			return domain.RootPage{Comments: []domain.Comment{c}, Next: &domain.RootCursor{ID: id, CreatedAt: stamp, Order: domain.RootASC}}, nil
		},
		search: func(ctx context.Context, o domain.SearchOptions) (domain.SearchPage, error) {
			check(ctx)
			if o.Query != " КОШКА " || o.Limit != 1 {
				t.Fatal("search parameters lost")
			}
			return domain.SearchPage{Matches: []domain.SearchMatch{{Comment: c, RootID: 1}}, Next: &domain.SearchCursor{ID: id, CreatedAt: stamp, Query: "кошка"}}, nil
		},
	}
	w := request(t, s, "POST", "/comments", `{"parent_id":"1","author":" Автор ","body":"Body"}`, 65536)
	if w.Code != nethttp.StatusCreated {
		t.Fatal(w.Code)
	}
	var dto commentDTO
	if err := json.Unmarshal(w.Body.Bytes(), &dto); err != nil {
		t.Fatal(err)
	}
	if dto.ID != "9223372036854775806" || dto.ParentID == nil || *dto.ParentID != "1" || !dto.CreatedAt.Equal(stamp) {
		t.Fatal("comment DTO lost data")
	}
	w = request(t, s, "DELETE", "/comments/9223372036854775806", "", 65536)
	if w.Code != nethttp.StatusNoContent || w.Body.Len() != 0 {
		t.Fatal("delete must have no response body")
	}
	w = request(t, s, "GET", "/comments?parent=9223372036854775806", "", 65536)
	if w.Code != nethttp.StatusOK {
		t.Fatal(w.Body.String())
	}
	w = request(t, s, "GET", "/comments?sort=oldest&limit=2", "", 65536)
	var page commentsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || page.NextCursor == nil {
		t.Fatal("missing root cursor")
	}
	rootCursor := *page.NextCursor
	s.roots = func(ctx context.Context, o domain.RootPageOptions) (domain.RootPage, error) {
		check(ctx)
		if o.After == nil || o.After.ID != id || !o.After.CreatedAt.Equal(stamp) || o.After.Order != domain.RootASC {
			t.Fatal("root cursor lost precision")
		}
		return domain.RootPage{}, nil
	}
	w = request(t, s, "GET", "/comments?sort=oldest&cursor="+url.QueryEscape(rootCursor), "", 65536)
	if w.Code != nethttp.StatusOK || !strings.Contains(w.Body.String(), `"comments":[]`) {
		t.Fatal("empty comments must be an array")
	}
	w = request(t, s, "GET", "/comments/search?q="+url.QueryEscape(" КОШКА ")+"&limit=1", "", 65536)
	var search searchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &search); err != nil || search.NextCursor == nil || search.Matches[0].RootID != "1" {
		t.Fatal("search DTO failed")
	}
	searchCursor := *search.NextCursor
	s.search = func(ctx context.Context, o domain.SearchOptions) (domain.SearchPage, error) {
		check(ctx)
		if o.After == nil || o.After.ID != id || !o.After.CreatedAt.Equal(stamp) || o.After.Query != "кошка" {
			t.Fatal("search cursor lost data")
		}
		return domain.SearchPage{}, nil
	}
	w = request(t, s, "GET", "/comments/search?q="+url.QueryEscape("кошка")+"&cursor="+url.QueryEscape(searchCursor), "", 65536)
	if w.Code != nethttp.StatusOK || !strings.Contains(w.Body.String(), `"matches":[]`) {
		t.Fatal("empty matches must be an array")
	}
	for _, path := range []string{"/comments?cursor=" + rootCursor, "/comments?sort=oldest&cursor=" + searchCursor, "/comments/search?q=dogs&cursor=" + searchCursor, "/comments/search?q=dogs&cursor=" + rootCursor} {
		if w := request(t, validatingStub(), "GET", path, "", 65536); w.Code != nethttp.StatusBadRequest {
			t.Fatal("cursor binding must be enforced")
		}
	}
}

func TestInvalidRequests(t *testing.T) {
	for _, path := range []string{"/comments?limit=0", "/comments?limit=-1", "/comments?limit=51", "/comments?limit=x", "/comments?limit=1&limit=2", "/comments?sort=asc", "/comments?sort=", "/comments?other=x", "/comments?parent=0", "/comments?parent=1&limit=1", "/comments?cursor=", "/comments?cursor=!!!", "/comments?cursor=" + strings.Repeat("A", 4097), "/comments/search", "/comments/search?q=+", "/comments/search?q=cats&sort=newest", "/comments/search?q=x&limit=51", "/comments?bad=%zz"} {
		t.Run(path, func(t *testing.T) {
			w := request(t, validatingStub(), "GET", path, "", 65536)
			if w.Code != nethttp.StatusBadRequest {
				t.Fatalf("got %d", w.Code)
			}
		})
	}
	for _, body := range []string{"", `null`, `[]`, `{}`, `{"author":"A","body":"B","unknown":1}`, `{"author":"A","body":"B"} {}`, `{"author":"A","author":"B","body":"C"}`, `{"author":null,"body":"B"}`, `{"author":"A","body":4}`, `{"author":"A","body":"B","parent_id":1}`, `{"author":"A","body":"B","parent_id":"0"}`, `{"author":"A","body":"\u0000"}`, "{\"author\":\"A\",\"body\":\"" + string([]byte{255}) + "\"}"} {
		if w := request(t, validatingStub(), "POST", "/comments", body, 65536); w.Code != nethttp.StatusBadRequest {
			t.Fatalf("bad JSON accepted, status %d", w.Code)
		}
	}
	if w := request(t, serviceStub{}, "POST", "/comments", `{"author":"A","body":"Body"}`, 2); w.Code != nethttp.StatusRequestEntityTooLarge {
		t.Fatal("body limit not enforced")
	}
	for _, id := range []string{"-1", "0", "+1", "x", "9223372036854775808"} {
		if w := request(t, serviceStub{}, "DELETE", "/comments/"+id, "", 65536); w.Code != nethttp.StatusBadRequest {
			t.Fatal("invalid path ID accepted")
		}
	}
	if w := request(t, serviceStub{}, "PUT", "/comments", "", 65536); w.Header().Get("Allow") != "GET, POST" {
		t.Fatal("nethttp.StatusMethodNotAllowed must list supported methods")
	}
	h, _ := New(serviceStub{})
	r := httptest.NewRequest("POST", "/comments", strings.NewReader("{}"))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != nethttp.StatusUnsupportedMediaType {
		t.Fatal("unsupported content type must fail")
	}
	for _, tc := range []struct {
		method, path string
		code         int
	}{{"GET", "/missing", nethttp.StatusNotFound}, {"PUT", "/comments", nethttp.StatusMethodNotAllowed}} {
		if w := request(t, serviceStub{}, tc.method, tc.path, "", 65536); w.Code != tc.code {
			t.Fatal("routing error status wrong")
		}
	}
}

func TestErrorMapping(t *testing.T) {
	for _, tc := range []struct {
		cause  error
		status int
		code   string
	}{
		{domain.ErrInvalidInput, nethttp.StatusBadRequest, "invalid_input"}, {domain.ErrNotFound, nethttp.StatusNotFound, "comment_not_found"}, {domain.ErrParentNotFound, nethttp.StatusNotFound, "parent_not_found"}, {domain.ErrResultTooLarge, nethttp.StatusRequestEntityTooLarge, "result_too_large"}, {domain.ErrConflict, nethttp.StatusConflict, "concurrent_conflict"}, {domain.ErrUnavailable, nethttp.StatusServiceUnavailable, "unavailable"}, {domain.ErrLockUnavailable, nethttp.StatusServiceUnavailable, "lock_unavailable"}, {domain.ErrQueryInterrupted, nethttp.StatusGatewayTimeout, "query_interrupted"}, {context.DeadlineExceeded, nethttp.StatusGatewayTimeout, "deadline_exceeded"}, {domain.ErrInternal, nethttp.StatusInternalServerError, "internal_error"}, {errors.New("secret"), nethttp.StatusInternalServerError, "internal_error"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			s := serviceStub{roots: func(context.Context, domain.RootPageOptions) (domain.RootPage, error) {
				return domain.RootPage{}, fmt.Errorf("private credentials: %w", tc.cause)
			}}
			w := request(t, s, "GET", "/comments", "", 65536)
			var result errorResponse
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if w.Code != tc.status || result.Error.Code != tc.code || strings.Contains(w.Body.String(), "private") || strings.Contains(w.Body.String(), "secret") {
				t.Fatal("unsafe or incorrect error response")
			}
			if tc.cause == domain.ErrLockUnavailable && w.Header().Get("Retry-After") != "1" {
				t.Fatal("lock error needs retry hint")
			}
		})
	}
	s := serviceStub{roots: func(context.Context, domain.RootPageOptions) (domain.RootPage, error) {
		return domain.RootPage{}, context.Canceled
	}}
	if w := request(t, s, "GET", "/comments", "", 65536); w.Body.Len() != 0 {
		t.Fatal("cancellation must not write a response")
	}
}

func TestCursorValidation(t *testing.T) {
	valid := cursorDTO{Version: 1, Kind: "roots", ID: "1", CreatedAt: time.Now(), Order: domain.RootDESC}
	for _, change := range []func(*cursorDTO){func(c *cursorDTO) { c.Version = 2 }, func(c *cursorDTO) { c.Kind = "search" }, func(c *cursorDTO) { c.ID = "0" }, func(c *cursorDTO) { c.CreatedAt = time.Time{} }, func(c *cursorDTO) { c.Query = "forbidden" }} {
		c := valid
		change(&c)
		data, _ := json.Marshal(c)
		if _, err := decodeCursor(base64.RawURLEncoding.EncodeToString(data), "roots"); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatal("bad cursor accepted")
		}
	}
	for _, raw := range []string{`{"version":1,"version":1}`, `{"version":1,"unknown":1}`, `null`, `{} {}`} {
		if _, err := decodeCursor(base64.RawURLEncoding.EncodeToString([]byte(raw)), "roots"); err == nil {
			t.Fatal("malformed cursor accepted")
		}
	}
	if _, err := New(nil); err == nil {
		t.Fatal("nil service accepted")
	}

}

func validatingStub() serviceStub {
	return serviceStub{
		create: func(_ context.Context, input domain.CreateCommentInput) (domain.Comment, error) {
			_, err := domain.NewCommentInput(input.ParentID, input.Author, input.Body)
			return domain.Comment{}, err
		},
		search: func(_ context.Context, options domain.SearchOptions) (domain.SearchPage, error) {
			_, err := options.NormalizedQuery()
			return domain.SearchPage{}, err
		},
	}
}

func TestJSONKeysAreCaseSensitive(t *testing.T) {
	for _, body := range []string{
		`{"Author":"A","body":"B"}`, `{"author":"A","Body":"B"}`, `{"AUTHOR":"A","body":"B"}`,
		`{"author":"A","body":"B","Parent_ID":null}`, `{"author":"A","AUTHOR":"B","body":"C"}`,
	} {
		if w := request(t, serviceStub{}, "POST", "/comments", body, 65536); w.Code != nethttp.StatusBadRequest {
			t.Fatal("noncanonical JSON key accepted")
		}
	}
	original := `{"version":1,"kind":"roots","created_at":"2025-01-02T03:04:05.123456789Z","id":"1","order":"desc"}`
	for _, key := range []string{"version", "kind", "created_at", "id", "order"} {
		data := strings.Replace(original, `"`+key+`":`, `"`+strings.ToUpper(key)+`":`, 1)
		if _, err := decodeCursor(base64.RawURLEncoding.EncodeToString([]byte(data)), "roots"); err == nil {
			t.Fatal("noncanonical cursor key accepted")
		}
	}
	search := strings.Replace(strings.Replace(original, `"roots"`, `"search"`, 1), `"order":"desc"`, `"QUERY":"cats"`, 1)
	if _, err := decodeCursor(base64.RawURLEncoding.EncodeToString([]byte(search)), "search"); err == nil {
		t.Fatal("noncanonical search cursor key accepted")
	}
}

func TestDomainValidationIsDelegated(t *testing.T) {
	createCalled, searchCalled := false, false
	s := serviceStub{
		create: func(_ context.Context, in domain.CreateCommentInput) (domain.Comment, error) {
			createCalled = true
			if in.Author != " " || in.Body != "" {
				t.Fatal("HTTP must preserve domain input")
			}
			return domain.Comment{}, domain.ErrInvalidInput
		},
		search: func(_ context.Context, o domain.SearchOptions) (domain.SearchPage, error) {
			searchCalled = true
			if o.Query != " " {
				t.Fatal("HTTP must preserve search text")
			}
			return domain.SearchPage{}, domain.ErrInvalidInput
		},
	}
	if w := request(t, s, "POST", "/comments", `{"author":" ","body":""}`, 65536); w.Code != nethttp.StatusBadRequest {
		t.Fatal("domain error status changed")
	}
	if w := request(t, s, "GET", "/comments/search?q=+", "", 65536); w.Code != nethttp.StatusBadRequest {
		t.Fatal("domain error status changed")
	}
	if !createCalled || !searchCalled {
		t.Fatal("domain validation must reach the service")
	}
}
