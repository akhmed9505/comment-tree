//go:build integration

package integration

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/akhmed9505/comment-tree/internal/domain"
	"github.com/akhmed9505/comment-tree/internal/infra/postgres"
	"github.com/akhmed9505/comment-tree/internal/repository/comments"
	"github.com/akhmed9505/comment-tree/tests/testutil"
)

func TestRootPagination(t *testing.T) {
	dsn := testutil.StartPostgres(t)
	testutil.Migrate(t, dsn, "up")
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	pool, err := postgres.NewPool(ctx, dsn, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	repo := comments.New(pool)
	for _, order := range []domain.RootOrder{domain.RootASC, domain.RootDESC} {
		page, err := repo.ListRoots(ctx, domain.RootPageOptions{Limit: 2, MaxNodes: 8, Order: order})
		if err != nil || len(page.Comments) != 0 || page.Next != nil {
			t.Fatal("empty database must return an empty terminal page")
		}
	}
	base := time.Date(2025, 2, 3, 4, 5, 6, 123456000, time.UTC)
	create := func(t *testing.T, parent *int64, stamp time.Time) domain.Comment {
		t.Helper()
		c, err := repo.Create(ctx, domain.CreateCommentInput{ParentID: parent, Author: "Автор", Body: "Body\nТекст"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, "UPDATE comments SET created_at = $1 WHERE id = $2", stamp, c.ID); err != nil {
			t.Fatal(err)
		}
		c.CreatedAt = stamp
		return c
	}
	roots := make([]domain.Comment, 5)
	trees := make(map[int64][]domain.Comment)
	for i, seconds := range []int{2, 1, 1, 3, 3} {
		root := create(t, nil, base.Add(time.Duration(seconds)*time.Second))
		roots[i] = root
		first := create(t, &root.ID, base.Add(10*time.Second))
		second := create(t, &root.ID, base.Add(-time.Second))
		grand := create(t, &first.ID, base.Add(20*time.Second))
		trees[root.ID] = []domain.Comment{root, second, first, grand}
	}
	ordered := []domain.Comment{roots[1], roots[2], roots[0], roots[3], roots[4]}
	for _, order := range []domain.RootOrder{domain.RootASC, domain.RootDESC} {
		for _, limit := range []int{1, 2, 5, domain.MaxRootPageSize} {
			t.Run(string(order)+"/"+strconv.Itoa(limit), func(t *testing.T) {
				expected := append([]domain.Comment(nil), ordered...)
				if order == domain.RootDESC {
					for i, j := 0, len(expected)-1; i < j; i, j = i+1, j-1 {
						expected[i], expected[j] = expected[j], expected[i]
					}
				}
				options := domain.RootPageOptions{Limit: limit, MaxNodes: 20, Order: order}
				seen := make(map[int64]bool)
				offset := 0
				for {
					page, err := repo.ListRoots(ctx, options)
					if err != nil {
						t.Fatal(err)
					}
					end := min(offset+limit, len(expected))
					var want []domain.Comment
					for _, root := range expected[offset:end] {
						want = append(want, trees[root.ID]...)
					}
					if len(page.Comments) != len(want) {
						t.Fatalf("got %d nodes, want %d", len(page.Comments), len(want))
					}
					for i, c := range page.Comments {
						w := want[i]
						if c.ID != w.ID || (c.ParentID == nil) != (w.ParentID == nil) || c.ParentID != nil && *c.ParentID != *w.ParentID || !c.CreatedAt.Equal(w.CreatedAt) || c.Author != w.Author || c.Body != w.Body {
							t.Fatal("page lost subtree data or ordering")
						}
						if seen[c.ID] {
							t.Fatal("duplicate node across pages")
						}
						seen[c.ID] = true
					}
					if end == len(expected) {
						if page.Next != nil {
							t.Fatal("last page must not have a cursor")
						}
						break
					}
					last := expected[end-1]
					if page.Next == nil || page.Next.ID != last.ID || !page.Next.CreatedAt.Equal(last.CreatedAt) || page.Next.Order != order {
						t.Fatal("cursor must refer to the last returned root in the requested order")
					}
					options.After = page.Next
					offset = end
				}
				if len(seen) != 20 {
					t.Fatal("pagination skipped nodes")
				}
				terminal := expected[len(expected)-1]
				options.After = &domain.RootCursor{ID: terminal.ID, CreatedAt: terminal.CreatedAt, Order: order}
				page, err := repo.ListRoots(ctx, options)
				if err != nil || len(page.Comments) != 0 || page.Next != nil {
					t.Fatal("cursor beyond the last root must produce an empty page")
				}
			})
		}
	}
	t.Run("node limits", func(t *testing.T) {
		for _, order := range []domain.RootOrder{domain.RootASC, domain.RootDESC} {
			options := domain.RootPageOptions{Limit: 2, MaxNodes: 8, Order: order}
			page, err := repo.ListRoots(ctx, options)
			if err != nil || len(page.Comments) != 8 || page.Next == nil {
				t.Fatal("exact node boundary must return complete subtrees")
			}
			for _, nodes := range []int{1, 7} {
				options.MaxNodes = nodes
				page, err = repo.ListRoots(ctx, options)
				if !errors.Is(err, domain.ErrResultTooLarge) || page.Comments != nil || page.Next != nil {
					t.Fatal("overflow must not expose a partial page or cursor")
				}
			}
			options.Limit = 1
			options.MaxNodes = 4
			page, err = repo.ListRoots(ctx, options)
			if err != nil || len(page.Comments) != 4 || page.Next == nil {
				t.Fatal("lookahead root and its subtree must not consume the page node budget")
			}
			options.After = page.Next
			options.Order = domain.RootOrder("invalid")
			page, err = repo.ListRoots(ctx, options)
			if !errors.Is(err, domain.ErrInvalidInput) || page.Comments != nil || page.Next != nil {
				t.Fatal("invalid order must not query or return a page")
			}
			options.Order = domain.RootASC
			if order == domain.RootASC {
				options.Order = domain.RootDESC
			}
			if _, err := repo.ListRoots(ctx, options); !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatal("cursor from another ordering must be rejected")
			}
		}
	})
	t.Run("deleted cursor boundary", func(t *testing.T) {
		options := domain.RootPageOptions{Limit: 1, MaxNodes: 20, Order: domain.RootASC}
		page, err := repo.ListRoots(ctx, options)
		if err != nil || page.Next == nil {
			t.Fatal("first page failed")
		}
		cursor := page.Next
		if err := repo.Delete(ctx, cursor.ID); err != nil {
			t.Fatal(err)
		}
		options.After = cursor
		options.Limit = domain.MaxRootPageSize
		page, err = repo.ListRoots(ctx, options)
		if err != nil || len(page.Comments) != 16 || page.Next != nil || page.Comments[0].ID != ordered[1].ID {
			t.Fatal("cursor must remain usable after its boundary root is deleted")
		}
	})
	t.Run("caller cancellation", func(t *testing.T) {
		canceled, stop := context.WithCancel(ctx)
		stop()
		page, err := repo.ListRoots(canceled, domain.RootPageOptions{Limit: 2, MaxNodes: 8, Order: domain.RootASC})
		if !errors.Is(err, context.Canceled) || page.Comments != nil || page.Next != nil {
			t.Fatal("cancellation must be preserved without partial data")
		}
	})
}
