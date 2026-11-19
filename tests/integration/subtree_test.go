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

func TestCommentSubtree(t *testing.T) {
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
	create := func(t *testing.T, parent *int64) domain.Comment {
		t.Helper()
		c, err := repo.Create(ctx, domain.CreateCommentInput{ParentID: parent, Author: "Автор", Body: "Hello\nПривет"})
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	assertComments := func(t *testing.T, got, want []domain.Comment) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("got %d comments, want %d", len(got), len(want))
		}
		expected := make(map[int64]domain.Comment, len(want))
		for _, c := range want {
			expected[c.ID] = c
		}
		seen := make(map[int64]bool, len(got))
		for _, c := range got {
			w, ok := expected[c.ID]
			if !ok || seen[c.ID] {
				t.Fatalf("unexpected or duplicate comment %d", c.ID)
			}
			seen[c.ID] = true
			if (c.ParentID == nil) != (w.ParentID == nil) || c.ParentID != nil && *c.ParentID != *w.ParentID || c.Author != w.Author || c.Body != w.Body || !c.CreatedAt.Equal(w.CreatedAt) {
				t.Fatalf("comment %d fields differ", c.ID)
			}
		}
		if got[0].ID != want[0].ID {
			t.Fatal("requested comment must be first")
		}
	}
	t.Run("single comment", func(t *testing.T) {
		root := create(t, nil)
		got, err := repo.GetSubtree(ctx, root.ID, 1)
		if err != nil {
			t.Fatal(err)
		}
		assertComments(t, got, []domain.Comment{root})
	})
	t.Run("branching tree and sibling order", func(t *testing.T) {
		root := create(t, nil)
		other := create(t, nil)
		create(t, &other.ID)
		children := []domain.Comment{create(t, &root.ID), create(t, &root.ID), create(t, &root.ID)}
		want := []domain.Comment{root}
		grandByParent := make(map[int64][]domain.Comment)
		base := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
		for i := range children {
			stamp := base
			if i == 0 {
				stamp = base.Add(time.Second)
			}
			children[i].CreatedAt = stamp
			if _, err := pool.Exec(ctx, "UPDATE comments SET created_at = $1 WHERE id = $2", stamp, children[i].ID); err != nil {
				t.Fatal(err)
			}
			want = append(want, children[i])
			grand := []domain.Comment{create(t, &children[i].ID), create(t, &children[i].ID)}
			// Backdate the later ID to make timestamp ordering differ from identity ordering.
			grand[0].CreatedAt = base.Add(3 * time.Second)
			grand[1].CreatedAt = base.Add(2 * time.Second)
			for _, c := range grand {
				if _, err := pool.Exec(ctx, "UPDATE comments SET created_at = $1 WHERE id = $2", c.CreatedAt, c.ID); err != nil {
					t.Fatal(err)
				}
			}
			grandByParent[children[i].ID] = grand
			want = append(want, grand...)
		}
		got, err := repo.GetSubtree(ctx, root.ID, len(want))
		if err != nil {
			t.Fatal(err)
		}
		assertComments(t, got, want)
		siblings := make(map[int64][]int64)
		for _, c := range got {
			if c.ParentID != nil {
				siblings[*c.ParentID] = append(siblings[*c.ParentID], c.ID)
			}
		}
		expectedOrder := []int64{children[1].ID, children[2].ID, children[0].ID}
		for i, id := range siblings[root.ID] {
			if id != expectedOrder[i] {
				t.Fatal("siblings must sort by timestamp, then ID")
			}
		}
		for _, c := range children {
			order := siblings[c.ID]
			grand := grandByParent[c.ID]
			if len(order) != 2 || order[0] != grand[1].ID || order[1] != grand[0].ID {
				t.Fatal("grandchildren must sort by timestamp")
			}
		}
		branch := children[1]
		got, err = repo.GetSubtree(ctx, branch.ID, 3)
		if err != nil {
			t.Fatal(err)
		}
		assertComments(t, got, append([]domain.Comment{branch}, grandByParent[branch.ID]...))
		for _, limit := range []int{1, len(want) - 1} {
			got, err = repo.GetSubtree(ctx, root.ID, limit)
			if !errors.Is(err, domain.ErrResultTooLarge) || got != nil {
				t.Fatalf("limit %d must fail without partial results, got %v", limit, err)
			}
		}
		got, err = repo.GetSubtree(ctx, root.ID, len(want))
		if err != nil {
			t.Fatal("pool must remain usable after a limit error")
		}
		assertComments(t, got, want)
	})
	t.Run("deep tree", func(t *testing.T) {
		const first = 10000
		const depth = 1024
		_, err := pool.Exec(ctx, `INSERT INTO comments (id, parent_id, author, body) OVERRIDING SYSTEM VALUE
 SELECT n, CASE WHEN n = $1 THEN NULL ELSE n - 1 END, 'Author', 'Body'
 FROM generate_series($1::bigint, $2::bigint) AS n`, int64(first), int64(first+depth-1))
		if err != nil {
			t.Fatal(err)
		}
		got, err := repo.GetSubtree(ctx, first, depth)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != depth {
			t.Fatalf("got %d nodes, want %d", len(got), depth)
		}
		for i, c := range got {
			if c.ID != int64(first+i) {
				t.Fatal("deep subtree has a missing or misordered node")
			}
			if i == 0 && c.ParentID != nil || i > 0 && (c.ParentID == nil || *c.ParentID != c.ID-1) {
				t.Fatal("deep subtree lost a parent link")
			}
		}
		got, err = repo.GetSubtree(ctx, first, depth-1)
		if !errors.Is(err, domain.ErrResultTooLarge) || got != nil {
			t.Fatal("deep tree must obey the node limit")
		}
	})
	t.Run("missing comment", func(t *testing.T) {
		got, err := repo.GetSubtree(ctx, 1<<60, 1)
		if !errors.Is(err, domain.ErrNotFound) || got != nil {
			t.Fatal("missing comment must return ErrNotFound without results")
		}
	})
	t.Run("caller cancellation", func(t *testing.T) {
		root := create(t, nil)
		canceled, stop := context.WithCancel(ctx)
		stop()
		got, err := repo.GetSubtree(canceled, root.ID, 1)
		if !errors.Is(err, context.Canceled) || got != nil {
			t.Fatal("caller cancellation must be preserved without results")
		}
		expired, stop := context.WithDeadline(ctx, time.Now().Add(-time.Second))
		defer stop()
		got, err = repo.GetSubtree(expired, root.ID, 1)
		if !errors.Is(err, context.DeadlineExceeded) || got != nil {
			t.Fatal("caller deadline must be preserved without results")
		}
	})
}
