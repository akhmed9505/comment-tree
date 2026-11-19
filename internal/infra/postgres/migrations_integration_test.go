//go:build integration

package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/akhmed9505/comment-tree/internal/domain"
	"github.com/akhmed9505/comment-tree/internal/testutil"
)

func TestCommentsMigration(t *testing.T) {
	dsn := testutil.StartPostgres(t)
	testutil.Migrate(t, dsn, "up")
	testutil.Migrate(t, dsn, "up")
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	cfg := testConfig()
	cfg.ConnectTimeout = 5 * time.Second
	pool, err := NewPool(ctx, dsn, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	insert := func(parent *int64) int64 {
		t.Helper()
		var id int64
		var created time.Time
		err := pool.QueryRow(ctx, "INSERT INTO comments (parent_id, author, body) VALUES ($1, 'Author', 'Body') RETURNING id, created_at", parent).Scan(&id, &created)
		if err != nil || id <= 0 || created.IsZero() {
			t.Fatal("insert comment with generated identity and timestamp failed")
		}
		return id
	}
	root := insert(nil)
	child := insert(&root)
	leaf := insert(&child)
	sibling := insert(&root)
	otherRoot := insert(nil)

	t.Run("domain text boundaries", func(t *testing.T) {
		for _, text := range []string{"я", "😀"} {
			author := strings.Repeat(text, domain.AuthorMaxLength)
			body := strings.Repeat(text, domain.BodyMaxLength)
			if _, err := pool.Exec(ctx, "INSERT INTO comments (author, body) VALUES ($1, $2)", author, body); err != nil {
				t.Fatal("Unicode boundary values must be accepted")
			}
			_, err := pool.Exec(ctx, "INSERT INTO comments (author, body) VALUES ($1, $2)", author+text, body)
			assertSQLState(t, err, "23514")
			_, err = pool.Exec(ctx, "INSERT INTO comments (author, body) VALUES ($1, $2)", author, body+text)
			assertSQLState(t, err, "23514")
		}
		for _, blank := range []string{"", " \t\n\r", "\u0085\u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000"} {
			_, err := pool.Exec(ctx, "INSERT INTO comments (author, body) VALUES ($1, 'Body')", blank)
			assertSQLState(t, err, "23514")
			_, err = pool.Exec(ctx, "INSERT INTO comments (author, body) VALUES ('Author', $1)", blank)
			assertSQLState(t, err, "23514")
		}
		for _, invalid := range []string{"bad\x00text", string([]byte{0xff})} {
			_, err := pool.Exec(ctx, "INSERT INTO comments (author, body) VALUES ($1, 'Body')", invalid)
			assertSQLState(t, err, "22021")
			_, err = pool.Exec(ctx, "INSERT INTO comments (author, body) VALUES ('Author', $1)", invalid)
			assertSQLState(t, err, "22021")
		}
	})

	t.Run("required fields and identity", func(t *testing.T) {
		for _, sql := range []string{
			"INSERT INTO comments (author, body) VALUES (NULL, 'Body')",
			"INSERT INTO comments (author, body) VALUES ('Author', NULL)",
			"INSERT INTO comments (author, body, created_at) VALUES ('Author', 'Body', NULL)",
		} {
			_, err := pool.Exec(ctx, sql)
			assertSQLState(t, err, "23502")
		}
		_, err := pool.Exec(ctx, "INSERT INTO comments (id, author, body) VALUES (999, 'Author', 'Body')")
		assertSQLState(t, err, "428C9")
		for _, id := range []int64{0, -1} {
			_, err := pool.Exec(ctx, "INSERT INTO comments (id, author, body) OVERRIDING SYSTEM VALUE VALUES ($1, 'Author', 'Body')", id)
			assertSQLState(t, err, "23514")
		}
		_, err = pool.Exec(ctx, "INSERT INTO comments (id, author, body) OVERRIDING SYSTEM VALUE VALUES ($1, 'Author', 'Body')", root)
		assertSQLState(t, err, "23505")
	})

	t.Run("tree integrity", func(t *testing.T) {
		_, err := pool.Exec(ctx, "UPDATE comments SET parent_id = 0 WHERE id = $1", leaf)
		assertSQLState(t, err, "23503")
		_, err = pool.Exec(ctx, "UPDATE comments SET parent_id = id WHERE id = $1", leaf)
		assertSQLState(t, err, "23514")
		_, err = pool.Exec(ctx, "UPDATE comments SET parent_id = $1 WHERE id = $2", leaf, root)
		assertSQLState(t, err, "23514")
		if _, err := pool.Exec(ctx, "DELETE FROM comments WHERE id = $1", child); err != nil {
			t.Fatal("delete subtree failed")
		}
		var removed, preserved int
		err = pool.QueryRow(ctx, "SELECT count(*) FILTER (WHERE id = ANY($1)), count(*) FILTER (WHERE id = ANY($2)) FROM comments", []int64{child, leaf}, []int64{root, sibling, otherRoot}).Scan(&removed, &preserved)
		if err != nil || removed != 0 || preserved != 3 {
			t.Fatal("cascade must delete only the selected subtree")
		}
		_, err = pool.Exec(ctx, "INSERT INTO comments (id, parent_id, author, body) OVERRIDING SYSTEM VALUE SELECT n, CASE WHEN n = 1000 THEN NULL ELSE n - 1 END, 'Author', 'Body' FROM generate_series(1000, 1255) AS n")
		if err != nil {
			t.Fatal("insert deep tree failed")
		}
		if _, err := pool.Exec(ctx, "DELETE FROM comments WHERE id = 1000"); err != nil {
			t.Fatal("delete deep tree failed")
		}
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM comments WHERE id >= 1000").Scan(&count); err != nil || count != 0 {
			t.Fatal("deep cascade left descendants")
		}
	})

	t.Run("indexes", func(t *testing.T) {
		var count int
		err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_indexes WHERE schemaname = 'public' AND tablename = 'comments'
		    AND (indexdef LIKE '%USING btree (created_at, id) WHERE (parent_id IS NULL)'
		      OR indexdef LIKE '%USING btree (parent_id, created_at, id) WHERE (parent_id IS NOT NULL)')`).Scan(&count)
		if err != nil || count != 2 {
			t.Fatal("missing root or reply ordering index")
		}
	})

	testutil.Migrate(t, dsn, "down")
	var absent bool
	err = pool.QueryRow(ctx, "SELECT to_regclass('public.comments') IS NULL AND to_regclass('public.comments_id_seq') IS NULL AND NOT EXISTS (SELECT 1 FROM pg_indexes WHERE schemaname = 'public' AND tablename = 'comments')").Scan(&absent)
	if err != nil || !absent {
		t.Fatal("rollback must remove table, identity sequence and indexes")
	}
	testutil.Migrate(t, dsn, "up")
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM comments").Scan(&count); err != nil || count != 0 {
		t.Fatal("reapplying migration must create an empty table")
	}
	insert(nil)
}
