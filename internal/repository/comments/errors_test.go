package comments

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/akhmed9505/comment-tree/internal/domain"
)

func TestMapError(t *testing.T) {
	for _, err := range []error{io.EOF, io.ErrUnexpectedEOF, &net.OpError{Op: "read", Net: "tcp", Err: errors.New("private diagnostic")}} {
		if got := mapError(t.Context(), fmt.Errorf("wrapped: %w", err)); got != domain.ErrUnavailable {
			t.Fatal("connection failure must be unavailable")
		}
	}
	for _, tc := range []struct {
		code, constraint string
		want             error
	}{
		{"23503", "comments_parent_id_fkey", domain.ErrInternal},
		{"23514", "comments_parent_precedes_child", domain.ErrInvalidInput},
		{"23503", "other_fkey", domain.ErrInternal},
		{"23514", "comments_parent_precedes_child", domain.ErrInvalidInput},
		{"23502", "", domain.ErrInvalidInput},
		{"23514", "comments_body_valid", domain.ErrInvalidInput},
		{"55P03", "", domain.ErrLockUnavailable},
		{"40P01", "", domain.ErrConflict},
		{"40001", "", domain.ErrConflict},
		{"57014", "", domain.ErrQueryInterrupted},
		{"08006", "", domain.ErrUnavailable},
		{"57P01", "", domain.ErrUnavailable},
		{"57P02", "", domain.ErrUnavailable},
		{"57P03", "", domain.ErrUnavailable},
		{"XX000", "", domain.ErrInternal},
	} {
		t.Run(tc.code+tc.constraint, func(t *testing.T) {
			raw := &pgconn.PgError{Code: tc.code, ConstraintName: tc.constraint, Message: "private diagnostic"}
			got := mapError(t.Context(), fmt.Errorf("wrapped: %w", raw))
			if got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			var leaked *pgconn.PgError
			if errors.As(got, &leaked) {
				t.Fatal("database diagnostics escaped")
			}
		})
	}
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		if got := mapError(t.Context(), fmt.Errorf("private: %w", cause)); got != cause {
			t.Fatal("context cause was lost")
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if got := mapError(ctx, &pgconn.PgError{Code: "57014"}); got != context.Canceled {
		t.Fatal("caller cancellation must take priority")
	}
	if got := mapError(t.Context(), errors.New("private diagnostic")); got != domain.ErrInternal {
		t.Fatal("unexpected error must be sanitized")
	}
}

func TestMapCreateError(t *testing.T) {
	for _, tc := range []struct {
		code, constraint string
		want             error
	}{
		{"23503", "comments_parent_id_fkey", domain.ErrParentNotFound},
		{"23514", "comments_parent_precedes_child", domain.ErrParentNotFound},
		{"23503", "other_fkey", domain.ErrInternal},
		{"23514", "comments_body_valid", domain.ErrInvalidInput},
		{"55P03", "", domain.ErrLockUnavailable},
	} {
		t.Run(tc.code+tc.constraint, func(t *testing.T) {
			raw := &pgconn.PgError{Code: tc.code, ConstraintName: tc.constraint, Message: "private diagnostic"}
			if got := mapCreateError(t.Context(), fmt.Errorf("wrapped: %w", raw)); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			if got := mapCreateError(ctx, raw); got != context.Canceled {
				t.Fatal("caller cancellation must take priority")
			}
		})
	}
	if got := mapCreateError(t.Context(), fmt.Errorf("private: %w", context.DeadlineExceeded)); got != context.DeadlineExceeded {
		t.Fatal("context cause was lost")
	}
}
