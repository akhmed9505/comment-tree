package comments

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
	"github.com/akhmed9505/comment-tree/internal/domain"
)

func TestCommentPageResponseBudgets(t *testing.T) {
	stamp := time.Date(2026, 11, 19, 1, 2, 3, 123456789, time.UTC)
	root := domain.Comment{ID: 9223372036854775806, Author: "Автор <>&", Body: strings.Repeat("Body\"\\\n\u2028\u2029", 20), CreatedAt: stamp}
	child := root
	child.ID++
	child.ParentID = &root.ID
	items := []domain.Comment{root, child}
	matches := []domain.SearchMatch{{Comment: child, RootID: root.ID}}
	rootCursor := &domain.RootCursor{ID: root.ID, CreatedAt: stamp, Order: domain.RootDESC}
	searchCursor := &domain.SearchCursor{ID: child.ID, CreatedAt: stamp, Query: "кот <>&"}
	rootNext, err := encodeCursor(cursorDTO{Kind: "roots", ID: "9223372036854775806", CreatedAt: stamp, Order: domain.RootDESC})
	if err != nil {
		t.Fatal(err)
	}
	searchNext, err := encodeCursor(cursorDTO{Kind: "search", ID: "9223372036854775807", CreatedAt: stamp, Query: searchCursor.Query})
	if err != nil {
		t.Fatal(err)
	}
	service := serviceStub{
		roots: func(context.Context, domain.RootPageOptions) (domain.RootPage, error) {
			return domain.RootPage{Comments: items, Next: rootCursor}, nil
		},
		subtree: func(context.Context, int64) ([]domain.Comment, error) { return items, nil },
		search: func(context.Context, domain.SearchOptions) (domain.SearchPage, error) {
			return domain.SearchPage{Matches: matches, Next: searchCursor}, nil
		},
	}
	for _, tc := range []struct {
		path string
		body any
	}{
		{"/comments", commentsResponse{Comments: []commentDTO{toComment(root), toComment(child)}, NextCursor: rootNext}},
		{"/comments?parent=9223372036854775806", commentsResponse{Comments: []commentDTO{toComment(root), toComment(child)}}},
		{"/comments/search?q=cats", searchResponse{Matches: []matchDTO{{Comment: toComment(child), RootID: "9223372036854775806"}}, NextCursor: searchNext}},
	} {
		t.Run(tc.path, func(t *testing.T) {
			want, err := json.Marshal(tc.body)
			if err != nil {
				t.Fatal(err)
			}
			want = append(want, '\n')
			for _, extra := range []int{-1, 0, 1} {
				router, err := testRoutes(service)
				if err != nil {
					t.Fatal(err)
				}
				limits := config.Limits{RequestTimeout: time.Second, MaxRequestBytes: 65536, MaxResponseBytes: int64(len(want) + extra), MaxConcurrentRequests: 1}
				controller, err := middleware.New(slog.New(slog.DiscardHandler), limits)
				if err != nil {
					t.Fatal(err)
				}
				w := httptest.NewRecorder()
				controller.Wrap(router).ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
				if err := controller.Shutdown(t.Context()); err != nil {
					t.Fatal(err)
				}
				if extra < 0 {
					var failure response.ErrorResponse
					if w.Code != http.StatusRequestEntityTooLarge || json.Unmarshal(w.Body.Bytes(), &failure) != nil || failure.Error.Code != "result_too_large" {
						t.Fatal("oversized page did not return a complete 413 error")
					}
				} else if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), want) {
					t.Fatal("page representation, ID, cursor or timestamp changed")
				}
				if int64(w.Body.Len()) > limits.MaxResponseBytes {
					t.Fatal("response exceeded its configured budget")
				}
			}
		})
	}
}

func TestLargeCommentTreeResponse(t *testing.T) {
	items := make([]domain.Comment, 10000)
	body := strings.Repeat("a", domain.BodyMaxLength)
	for i := range items {
		items[i] = domain.Comment{ID: int64(i + 1), Author: "Author", Body: body, CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
		if i > 0 {
			items[i].ParentID = &items[0].ID
		}
	}
	service := serviceStub{
		roots: func(context.Context, domain.RootPageOptions) (domain.RootPage, error) {
			return domain.RootPage{Comments: items}, nil
		},
		subtree: func(context.Context, int64) ([]domain.Comment, error) { return items, nil },
	}
	for _, path := range []string{"/comments", "/comments?parent=1"} {
		w := request(t, service, http.MethodGet, path, "", 65536)
		var failure response.ErrorResponse
		if w.Code != http.StatusRequestEntityTooLarge || json.Unmarshal(w.Body.Bytes(), &failure) != nil || failure.Error.Code != "result_too_large" {
			t.Fatal("large comment tree did not return a complete 413 response")
		}
	}
}
