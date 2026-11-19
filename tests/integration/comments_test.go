//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/akhmed9505/comment-tree/internal/domain"
	"github.com/akhmed9505/comment-tree/internal/infra/postgres"
	"github.com/akhmed9505/comment-tree/internal/repository/comments"
	"github.com/akhmed9505/comment-tree/tests/testutil"
)

func TestCommentsRepository(t *testing.T) {
	dsn := testutil.StartPostgres(t)
	testutil.Migrate(t, dsn, "up")
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	cfg := testConfig()
	cfg.MaxConns = 4
	pool, err := postgres.NewPool(ctx, dsn, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	repo := comments.New(pool)
	create := func(t *testing.T, parent *int64) domain.Comment {
		t.Helper()
		c, err := repo.Create(ctx, domain.CreateCommentInput{ParentID: parent, Author: "Автор", Body: "Text '); DROP TABLE comments; --"})
		if err != nil {
			t.Fatal(err)
		}
		if c.ID <= 0 || c.CreatedAt.IsZero() || c.Author != "Автор" || c.Body != "Text '); DROP TABLE comments; --" {
			t.Fatal("stored comment differs from input")
		}
		if (c.ParentID == nil) != (parent == nil) || parent != nil && *c.ParentID != *parent {
			t.Fatal("parent was not returned")
		}
		return c
	}
	root := create(t, nil)
	child := create(t, &root.ID)
	leaf := create(t, &child.ID)
	sibling := create(t, &root.ID)
	other := create(t, nil)
	parent := int64(1 << 60)
	_, err = repo.Create(ctx, domain.CreateCommentInput{ParentID: &parent, Author: "A", Body: "B"})
	if !errors.Is(err, domain.ErrParentNotFound) {
		t.Fatalf("expected missing future parent, got %v", err)
	}
	if err := repo.Delete(ctx, child.ID); err != nil {
		t.Fatal(err)
	}
	_, err = repo.Create(ctx, domain.CreateCommentInput{ParentID: &child.ID, Author: "A", Body: "B"})
	if !errors.Is(err, domain.ErrParentNotFound) {
		t.Fatalf("expected missing older parent, got %v", err)
	}
	var removed, preserved int
	err = pool.QueryRow(ctx, "SELECT count(*) FILTER (WHERE id = ANY($1)), count(*) FILTER (WHERE id = ANY($2)) FROM comments", []int64{child.ID, leaf.ID}, []int64{root.ID, sibling.ID, other.ID}).Scan(&removed, &preserved)
	if err != nil || removed != 0 || preserved != 3 {
		t.Fatal("subtree deletion affected wrong rows")
	}
	if err := repo.Delete(ctx, child.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("deleted comment must be missing")
	}
	if err := repo.Delete(ctx, 1<<60); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("unknown comment must be missing")
	}
	if err := repo.Delete(ctx, root.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(ctx, sibling.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("root deletion must cascade")
	}

	t.Run("concurrent create and delete", func(t *testing.T) {
		for range 30 {
			parent := create(t, nil)
			start := make(chan struct{})
			var wg sync.WaitGroup
			var reply domain.Comment
			var createErr, deleteErr error
			wg.Add(2)
			go func() {
				defer wg.Done()
				<-start
				reply, createErr = repo.Create(ctx, domain.CreateCommentInput{ParentID: &parent.ID, Author: "A", Body: "B"})
			}()
			go func() { defer wg.Done(); <-start; deleteErr = repo.Delete(ctx, parent.ID) }()
			close(start)
			wg.Wait()
			if deleteErr != nil {
				t.Fatal(deleteErr)
			}
			if createErr != nil && !errors.Is(createErr, domain.ErrParentNotFound) {
				t.Fatalf("unexpected concurrent creation error: %v", createErr)
			}
			var count int
			if err := pool.QueryRow(ctx, "SELECT count(*) FROM comments WHERE id = $1 OR id = $2 OR parent_id = $1", parent.ID, reply.ID).Scan(&count); err != nil || count != 0 {
				t.Fatal("concurrent delete left parent or reply")
			}
		}
	})
	t.Run("lock and statement timeouts", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		if _, err := tx.Exec(ctx, "SELECT id FROM comments WHERE id = $1 FOR UPDATE", other.ID); err != nil {
			t.Fatal(err)
		}
		if err := repo.Delete(ctx, other.ID); !errors.Is(err, domain.ErrLockUnavailable) {
			t.Fatalf("expected lock timeout, got %v", err)
		}
		if _, err := tx.Exec(ctx, `CREATE FUNCTION pg_temp.delay_delete() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_sleep(5); RETURN OLD; END $$`); err != nil {
			t.Fatal(err)
		}
		// The trigger is visible only after commit; it exercises server cancellation during repository SQL.
		if _, err := tx.Exec(ctx, `CREATE TRIGGER delay_delete BEFORE DELETE ON comments FOR EACH ROW EXECUTE FUNCTION pg_temp.delay_delete()`); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err := repo.Delete(ctx, other.ID); !errors.Is(err, domain.ErrQueryInterrupted) {
			t.Fatalf("expected statement timeout, got %v", err)
		}
		if _, err := pool.Exec(ctx, "DROP TRIGGER delay_delete ON comments"); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("caller cancellation", func(t *testing.T) {
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		err := repo.Delete(canceled, other.ID)
		if !errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "postgres://") {
			t.Fatal("cancellation must be preserved safely")
		}
	})
}
