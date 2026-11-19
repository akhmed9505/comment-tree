//go:build integration

package integration

import (
	"errors"
	"io"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/testcontainers/testcontainers-go"
	tcexec "github.com/testcontainers/testcontainers-go/exec"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/akhmed9505/comment-tree/internal/domain"
	"github.com/akhmed9505/comment-tree/internal/infra/postgres"
	"github.com/akhmed9505/comment-tree/internal/repository/comments"
	"github.com/akhmed9505/comment-tree/tests/testutil"
)

func TestDatabasePermissions(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		name := "fresh database"
		if legacy {
			name = "existing database"
		}
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			files := []testcontainers.ContainerFile{}
			for _, name := range []string{"setup.sh", "setup.sql", "grants.sql"} {
				files = append(files, testcontainers.ContainerFile{
					HostFilePath:      filepath.Join("..", "..", "docker", "postgres", name),
					ContainerFilePath: "/postgres/" + name, FileMode: 0644,
				})
			}
			container, err := tcpostgres.Run(ctx, "postgres:18.6-alpine",
				tcpostgres.WithDatabase("permissions"), tcpostgres.WithUsername("admin_role"),
				tcpostgres.WithPassword("admin_password"), tcpostgres.BasicWaitStrategies(),
				testcontainers.WithFiles(files...), testcontainers.WithEnv(map[string]string{
					"PGHOST": "127.0.0.1", "PGDATABASE": "permissions", "PGUSER": "admin_role", "PGPASSWORD": "admin_password",
					"MIGRATION_USER": "migration_role", "MIGRATION_PASSWORD": "migration'password\\test",
					"APP_DB_USER": "application_role", "APP_DB_PASSWORD": "application'password\\test",
				}),
			)
			testcontainers.CleanupContainer(t, container)
			if err != nil {
				t.Fatal("start permissions database failed")
			}
			adminDSN, err := container.ConnectionString(ctx, "sslmode=disable")
			if err != nil {
				t.Fatal("get permissions connection failed")
			}
			roleDSN := func(user, password string) string {
				parsed, err := url.Parse(adminDSN)
				if err != nil {
					t.Fatal("parse permissions connection failed")
				}
				parsed.User = url.UserPassword(user, password)
				return parsed.String()
			}
			run := func(command []string, env ...string) {
				t.Helper()
				code, output, err := container.Exec(ctx, command, tcexec.WithEnv(env))
				if output != nil {
					_, _ = io.Copy(io.Discard, output)
				}
				if err != nil || code != 0 {
					t.Fatal("database provisioning failed; diagnostics omitted to protect credentials")
				}
			}
			admin, err := postgres.NewPool(ctx, adminDSN, testConfig())
			if err != nil {
				t.Fatal(err)
			}
			defer admin.Close()
			var preserved int64
			if legacy {
				testutil.Migrate(t, adminDSN, "up")
				if err := admin.QueryRow(ctx, "INSERT INTO comments(author, body) VALUES ('Existing', 'Preserved') RETURNING id").Scan(&preserved); err != nil {
					t.Fatal("seed existing database failed")
				}
			}
			run([]string{"/bin/sh", "/postgres/setup.sh"})
			migrationDSN := roleDSN("migration_role", "migration'password\\test")
			testutil.Migrate(t, migrationDSN, "up")
			grant := func() {
				run([]string{"psql", "-X", "-v", "ON_ERROR_STOP=1", "-f", "/postgres/grants.sql"},
					"PGUSER=migration_role", "PGPASSWORD=migration'password\\test")
			}
			grant()
			app, err := postgres.NewPool(ctx, roleDSN("application_role", "application'password\\test"), testConfig())
			if err != nil {
				t.Fatal(err)
			}
			defer app.Close()
			repo := comments.New(app)
			root, err := repo.Create(ctx, domain.CreateCommentInput{Author: "Author", Body: "Кошки running"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := repo.Create(ctx, domain.CreateCommentInput{ParentID: &root.ID, Author: "Reply", Body: "Answer"}); err != nil {
				t.Fatal(err)
			}
			if subtree, err := repo.GetSubtree(ctx, root.ID, 10); err != nil || len(subtree) != 2 {
				t.Fatal("application cannot read subtree")
			}
			if _, err := repo.ListRoots(ctx, domain.RootPageOptions{Limit: 10, MaxNodes: 20, Order: domain.RootDESC}); err != nil {
				t.Fatal(err)
			}
			if page, err := repo.Search(ctx, domain.SearchOptions{Query: "кошка run", Limit: 10}); err != nil || len(page.Matches) != 1 {
				t.Fatal("application cannot search")
			}
			for _, query := range []string{
				"UPDATE comments SET body = 'changed'", "TRUNCATE comments", "DROP TABLE comments",
				"ALTER TABLE comments ADD COLUMN forbidden text", "CREATE TABLE public.forbidden(id int)",
				"CREATE TEMP TABLE forbidden(id int)", "CREATE DATABASE forbidden", "CREATE ROLE forbidden",
				"SELECT * FROM goose_db_version", "SELECT setval('comments_id_seq', 1)",
				"SELECT nextval(pg_get_serial_sequence('public.goose_db_version', 'id'))",
				"SET ROLE migration_role", "SET ROLE admin_role",
			} {
				_, err := app.Exec(ctx, query)
				var pgErr *pgconn.PgError
				if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
					t.Fatalf("expected insufficient privilege for %q", query)
				}
			}
			if err := repo.Delete(ctx, root.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.GetSubtree(ctx, root.ID, 10); !errors.Is(err, domain.ErrNotFound) {
				t.Fatal("cascade deletion failed")
			}
			// Reconfiguration and Goose rollback must preserve the runtime role's privileges.
			run([]string{"/bin/sh", "/postgres/setup.sh"})
			testutil.Migrate(t, migrationDSN, "down")
			testutil.Migrate(t, migrationDSN, "up")
			grant()
			if legacy {
				if got, err := repo.GetSubtree(ctx, preserved, 10); err != nil || got[0].Body != "Preserved" {
					t.Fatal("existing data was not preserved")
				}
			}
			var elevated bool
			if err := app.QueryRow(ctx, "SELECT rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls FROM pg_roles WHERE rolname=current_user").Scan(&elevated); err != nil || elevated {
				t.Fatal("application role has elevated attributes")
			}
			var migrationElevated bool
			if err := admin.QueryRow(ctx, "SELECT rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls FROM pg_roles WHERE rolname='migration_role'").Scan(&migrationElevated); err != nil || migrationElevated {
				t.Fatal("migration role has elevated attributes")
			}
			testutil.Migrate(t, migrationDSN, "down")
			testutil.Migrate(t, migrationDSN, "down")
			testutil.Migrate(t, migrationDSN, "up")
			grant()
			if _, err := repo.Create(ctx, domain.CreateCommentInput{Author: "Restored", Body: "After rollback"}); err != nil {
				t.Fatal("runtime privileges were not restored after migrations")
			}
			for _, user := range []string{"admin_role", strings.Repeat("a", 64)} {
				code, output, err := container.Exec(ctx, []string{"/bin/sh", "/postgres/setup.sh"}, tcexec.WithEnv([]string{"APP_DB_USER=" + user}))
				if output != nil {
					_, _ = io.Copy(io.Discard, output)
				}
				if err != nil || code == 0 {
					t.Fatal("invalid role settings accepted")
				}
			}

		})
	}
}
