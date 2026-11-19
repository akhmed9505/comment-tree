package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	comments "github.com/akhmed9505/comment-tree/internal/delivery/http/handler/comments"
	"github.com/akhmed9505/comment-tree/internal/delivery/http/handler/health"
	"github.com/akhmed9505/comment-tree/internal/domain"
)

type routingService struct{ last string }

func (s *routingService) Create(context.Context, domain.CreateCommentInput) (domain.Comment, error) {
	s.last = "create"
	return domain.Comment{ID: 1, CreatedAt: time.Now()}, nil
}

func (s *routingService) Delete(context.Context, int64) error { s.last = "delete"; return nil }

func (s *routingService) GetSubtree(context.Context, int64) ([]domain.Comment, error) {
	s.last = "subtree"
	return nil, nil
}

func (s *routingService) ListRoots(context.Context, domain.RootPageOptions) (domain.RootPage, error) {
	s.last = "roots"
	return domain.RootPage{}, nil
}

func (s *routingService) Search(context.Context, domain.SearchOptions) (domain.SearchPage, error) {
	s.last = "search"
	return domain.SearchPage{}, nil
}

func TestRoutes(t *testing.T) {
	service := &routingService{}
	c, _ := comments.New(service)
	h, _ := health.New(func(context.Context) error { return nil }, time.Second)
	router := NewRouter(c, h)
	for _, tc := range []struct {
		method, path, last string
		status             int
	}{
		{http.MethodPost, "/comments", "create", http.StatusCreated},
		{http.MethodGet, "/comments", "roots", http.StatusOK},
		{http.MethodGet, "/comments?parent=1", "subtree", http.StatusOK},
		{http.MethodDelete, "/comments/1", "delete", http.StatusNoContent},
		{http.MethodGet, "/comments/search?q=cats", "search", http.StatusOK},
		{http.MethodGet, "/", "", http.StatusOK},
		{http.MethodGet, "/assets/styles.css", "", http.StatusOK},
		{http.MethodGet, "/assets/app.js", "", http.StatusOK},
		{http.MethodHead, "/", "", http.StatusOK},
		{http.MethodHead, "/assets/styles.css", "", http.StatusOK},
		{http.MethodHead, "/assets/app.js", "", http.StatusOK},
		{http.MethodGet, "/assets/missing.js", "", http.StatusNotFound},
		{http.MethodGet, "/healthz", "", http.StatusOK},
		{http.MethodGet, "/readyz", "", http.StatusOK},
		{http.MethodPut, "/comments", "", http.StatusMethodNotAllowed},
		{http.MethodPost, "/healthz", "", http.StatusMethodNotAllowed},
		{http.MethodGet, "/missing", "", http.StatusNotFound},
	} {
		service.last = ""
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{"author":"A","body":"B"}`))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tc.status || service.last != tc.last {
			t.Fatalf("route %s %s got status %d and handler %s", tc.method, tc.path, w.Code, service.last)
		}
		if tc.method == http.MethodHead && w.Body.Len() != 0 {
			t.Fatal("HEAD returned an asset body")
		}
		if tc.status == http.StatusMethodNotAllowed && w.Header().Get("Allow") == "" {
			t.Fatal("405 missing Allow")
		}
	}
}
