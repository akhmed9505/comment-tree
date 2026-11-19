//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/akhmed9505/comment-tree/internal/domain"
	"github.com/akhmed9505/comment-tree/internal/infra/postgres"
	"github.com/akhmed9505/comment-tree/internal/repository/comments"
	"github.com/akhmed9505/comment-tree/tests/testutil"
)

func TestCommentSearch(t *testing.T) {
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
	create := func(t *testing.T, parent *int64, body string) domain.Comment {
		t.Helper()
		c, err := repo.Create(ctx, domain.CreateCommentInput{ParentID: parent, Author: "AuthorOnlyTerm", Body: body})
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	root := create(t, nil, "Обычное обсуждение")
	child := create(t, &root.ID, "Пушистые кошки гуляют рядом с dogs running")
	leaf := create(t, &child.ID, "Кошка гуляет и dog runs")
	other := create(t, nil, "Кошки спят")
	english := create(t, nil, "The dogs are running quickly")
	matches := map[int64]domain.Comment{root.ID: root, child.ID: child, leaf.ID: leaf, other.ID: other, english.ID: english}
	for _, tc := range []struct {
		query string
		ids   []int64
	}{
		{"кошкой", []int64{other.ID, leaf.ID, child.ID}},
		{"dog run", []int64{english.ID, leaf.ID, child.ID}},
		{"КОШКОЙ running", []int64{leaf.ID, child.ID}},
		{"кошка гулять", []int64{leaf.ID, child.ID}},
		{"кошка quickly", nil},
		{"AuthorOnlyTerm", nil},
		{"the and of", nil},
		{"и в на", nil},
		{"!!! & | :*", nil},
		{"'); DROP TABLE comments; --", nil},
	} {
		t.Run(tc.query, func(t *testing.T) {
			page, err := repo.Search(ctx, domain.SearchOptions{Query: tc.query, Limit: 50})
			if err != nil {
				t.Fatal(err)
			}
			if len(page.Matches) != len(tc.ids) || page.Next != nil {
				t.Fatalf("got %d matches, want %d", len(page.Matches), len(tc.ids))
			}
			for i, hit := range page.Matches {
				want := matches[tc.ids[i]]
				expectedRoot := want.ID
				if want.ParentID != nil {
					expectedRoot = root.ID
				}
				if hit.Comment.ID != want.ID || hit.RootID != expectedRoot || hit.Comment.Author != want.Author || hit.Comment.Body != want.Body || !hit.Comment.CreatedAt.Equal(want.CreatedAt) || (hit.Comment.ParentID == nil) != (want.ParentID == nil) || hit.Comment.ParentID != nil && *hit.Comment.ParentID != *want.ParentID {
					t.Fatal("search returned unexpected comment data or discussion root")
				}
			}
		})
	}
	t.Run("pagination", func(t *testing.T) {
		base := time.Date(2025, 1, 2, 3, 4, 5, 123456000, time.UTC)
		for _, c := range []domain.Comment{child, leaf, english} {
			stamp := base
			if c.ID == child.ID {
				stamp = base.Add(time.Second)
			}
			if _, err := pool.Exec(ctx, "UPDATE comments SET created_at=$1 WHERE id=$2", stamp, c.ID); err != nil {
				t.Fatal(err)
			}
		}
		expected := []int64{child.ID, english.ID, leaf.ID}
		for _, limit := range []int{1, 2, 3, 50} {
			options := domain.SearchOptions{Query: " DOG\tRUN ", Limit: limit}
			var got []int64
			for {
				page, err := repo.Search(ctx, options)
				if err != nil {
					t.Fatal(err)
				}
				for _, hit := range page.Matches {
					got = append(got, hit.Comment.ID)
				}
				if page.Next == nil {
					break
				}
				last := page.Matches[len(page.Matches)-1].Comment
				if page.Next.ID != last.ID || !page.Next.CreatedAt.Equal(last.CreatedAt) || page.Next.Query != "dog run" {
					t.Fatal("cursor must refer to the last returned match and normalized query")
				}
				options.After = page.Next
			}
			if len(got) != len(expected) {
				t.Fatal("pagination skipped or duplicated matches")
			}
			for i, id := range got {
				if id != expected[i] {
					t.Fatal("search must use descending timestamp and ID ordering")
				}
			}
		}
		cursor := &domain.SearchCursor{ID: leaf.ID, CreatedAt: base, Query: "dog run"}
		page, err := repo.Search(ctx, domain.SearchOptions{Query: "dog run", Limit: 1, After: cursor})
		if err != nil || len(page.Matches) != 0 || page.Next != nil {
			t.Fatal("cursor after the last match must return an empty terminal page")
		}
		if _, err := repo.Search(ctx, domain.SearchOptions{Query: "кошка", Limit: 1, After: cursor}); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatal("cursor from another query must be rejected")
		}
	})
	t.Run("vector updates", func(t *testing.T) {
		c := create(t, nil, "Olduniqueterm")
		if _, err := pool.Exec(ctx, "UPDATE comments SET body='Newuniqueterm' WHERE id=$1", c.ID); err != nil {
			t.Fatal(err)
		}
		old, err := repo.Search(ctx, domain.SearchOptions{Query: "Olduniqueterm", Limit: 1})
		if err != nil {
			t.Fatal(err)
		}
		fresh, err := repo.Search(ctx, domain.SearchOptions{Query: "Newuniqueterm", Limit: 1})
		if err != nil {
			t.Fatal(err)
		}
		if len(old.Matches) != 0 || len(fresh.Matches) != 1 || fresh.Matches[0].Comment.ID != c.ID {
			t.Fatal("stored vector must follow body updates")
		}
	})
	t.Run("deep discussion root", func(t *testing.T) {
		_, err := pool.Exec(ctx, `INSERT INTO comments (id,parent_id,author,body) OVERRIDING SYSTEM VALUE
 SELECT n, CASE WHEN n=10000 THEN NULL ELSE n-1 END, 'Author', CASE WHEN n=11023 THEN 'Deepuniqueterm' ELSE 'Ancestor' END FROM generate_series(10000,11023) n`)
		if err != nil {
			t.Fatal(err)
		}
		page, err := repo.Search(ctx, domain.SearchOptions{Query: "Deepuniqueterm", Limit: 1})
		if err != nil || len(page.Matches) != 1 || page.Matches[0].RootID != 10000 || page.Matches[0].Comment.ID != 11023 {
			t.Fatalf("deep search root resolution failed: %v", err)
		}
	})
	t.Run("cancellation", func(t *testing.T) {
		canceled, stop := context.WithCancel(ctx)
		stop()
		page, err := repo.Search(canceled, domain.SearchOptions{Query: "dog", Limit: 1})
		if !errors.Is(err, context.Canceled) || page.Matches != nil || page.Next != nil {
			t.Fatal("cancellation must not return partial matches")
		}
	})
}

func TestSearchMigration(t *testing.T) {
	dsn := testutil.StartPostgres(t)
	testutil.Migrate(t, dsn, "up-by-one")
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	pool, err := postgres.NewPool(ctx, dsn, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var id int64
	if err := pool.QueryRow(ctx, "INSERT INTO comments (author,body) VALUES ('Author','Кошки running') RETURNING id").Scan(&id); err != nil {
		t.Fatal(err)
	}
	testutil.Migrate(t, dsn, "up")
	var found bool
	if err := pool.QueryRow(ctx, "SELECT search_vector @@ plainto_tsquery('public.comment_search', 'кошкой run') FROM comments WHERE id=$1", id).Scan(&found); err != nil || !found {
		t.Fatal("search migration must backfill existing comments")
	}
	var indexed bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE tablename='comments' AND indexname='comments_search_vector_idx' AND indexdef LIKE '%USING gin (search_vector)%')").Scan(&indexed); err != nil || !indexed {
		t.Fatal("search vector must have a GIN index")
	}
	testutil.Migrate(t, dsn, "down")
	var removed bool
	if err := pool.QueryRow(ctx, `SELECT NOT EXISTS (SELECT 1 FROM pg_ts_config c JOIN pg_namespace n ON n.oid=c.cfgnamespace WHERE n.nspname='public' AND c.cfgname='comment_search')
 AND to_regclass('public.comments_search_vector_idx') IS NULL
 AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='comments' AND column_name='search_vector')`).Scan(&removed); err != nil || !removed {
		t.Fatal("search rollback must remove its configuration, column and index")
	}
	var body string
	if err := pool.QueryRow(ctx, "SELECT body FROM comments WHERE id=$1", id).Scan(&body); err != nil || body != "Кошки running" {
		t.Fatal("search rollback must preserve comments")
	}
	testutil.Migrate(t, dsn, "up")
	if err := pool.QueryRow(ctx, "SELECT search_vector @@ plainto_tsquery('public.comment_search', 'кошкой run') FROM comments WHERE id=$1", id).Scan(&found); err != nil || !found {
		t.Fatal("reapplied search migration must restore the vector")
	}
}
