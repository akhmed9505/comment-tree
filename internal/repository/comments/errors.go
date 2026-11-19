package comments

import (
	"context"
	"errors"
	"io"
	"net"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/akhmed9505/comment-tree/internal/domain"
)

func mapError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		if errors.Is(err, cause) {
			return cause
		}
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23502", "23514", "22021", "22001":
			return domain.ErrInvalidInput
		case "55P03":
			return domain.ErrLockUnavailable
		case "40P01", "40001":
			return domain.ErrConflict
		case "57014":
			return domain.ErrQueryInterrupted
		case "57P01", "57P02", "57P03":
			return domain.ErrUnavailable
		}
		if len(pgErr.Code) >= 2 && pgErr.Code[:2] == "08" {
			return domain.ErrUnavailable
		}
	}
	var netErr net.Error
	var connectErr *pgconn.ConnectError
	if errors.As(err, &netErr) || errors.As(err, &connectErr) ||
		errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || pgconn.SafeToRetry(err) {
		return domain.ErrUnavailable
	}
	return domain.ErrInternal
}

func mapCreateError(ctx context.Context, err error) error {
	mapped := mapError(ctx, err)
	if mapped != domain.ErrInternal && mapped != domain.ErrInvalidInput {
		return mapped
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		// A missing parent can fail the ordering check before the foreign key check.
		if (pgErr.Code == "23503" && pgErr.ConstraintName == "comments_parent_id_fkey") ||
			(pgErr.Code == "23514" && pgErr.ConstraintName == "comments_parent_precedes_child") {
			return domain.ErrParentNotFound
		}
	}
	return mapped
}
