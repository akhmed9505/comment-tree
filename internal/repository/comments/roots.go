package comments

import (
	"context"
	"fmt"
	"math"

	"github.com/akhmed9505/comment-tree/internal/domain"
)

// ListRoots returns complete subtrees grouped by root, with descendants ordered chronologically.
// A single statement keeps root selection and recursive reads in the same snapshot.
func (r *Repository) ListRoots(ctx context.Context, options domain.RootPageOptions) (domain.RootPage, error) {
	if err := validateRootPage(options); err != nil {
		return domain.RootPage{}, err
	}
	direction, comparison := "ASC", ">"
	if options.Order == domain.RootDESC {
		direction, comparison = "DESC", "<"
	}
	boundary := ""
	args := []any{options.Limit, options.MaxNodes + 1}
	if options.After != nil {
		boundary = "AND (created_at, id) " + comparison + " ($3::timestamptz, $4::bigint)"
		args = append(args, options.After.CreatedAt, options.After.ID)
	}
	// Only validated SQL fragments are interpolated; cursor values remain parameters.
	query := fmt.Sprintf(`WITH RECURSIVE candidates AS MATERIALIZED (
		SELECT id, parent_id, author, body, created_at FROM comments
		WHERE parent_id IS NULL %s
		ORDER BY created_at %s, id %s LIMIT $1::bigint + 1
	), roots AS (
		SELECT *, row_number() OVER (ORDER BY created_at %s, id %s) AS position
		FROM candidates ORDER BY created_at %s, id %s LIMIT $1
	), tree AS (
		SELECT * FROM roots
		UNION ALL
		SELECT c.id, c.parent_id, c.author, c.body, c.created_at, p.position
		FROM comments c JOIN tree p ON c.parent_id = p.id
	)
	SELECT id, parent_id, author, body, created_at,
		(SELECT count(*) > $1 FROM candidates) AS has_next
	FROM (SELECT * FROM tree LIMIT $2) bounded
	ORDER BY position, (parent_id IS NULL) DESC, created_at ASC, id ASC`,
		boundary, direction, direction, direction, direction, direction, direction)
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return domain.RootPage{}, fmt.Errorf("list comment roots: %w", mapError(ctx, err))
	}
	defer rows.Close()
	var page domain.RootPage
	var lastRoot domain.Comment
	var hasNext bool
	for rows.Next() {
		var comment domain.Comment
		if err := rows.Scan(&comment.ID, &comment.ParentID, &comment.Author, &comment.Body, &comment.CreatedAt, &hasNext); err != nil {
			return domain.RootPage{}, fmt.Errorf("scan root page: %w", mapError(ctx, err))
		}
		page.Comments = append(page.Comments, comment)
		if comment.ParentID == nil {
			lastRoot = comment
		}
	}
	if err := rows.Err(); err != nil {
		return domain.RootPage{}, fmt.Errorf("read root page: %w", mapError(ctx, err))
	}
	if len(page.Comments) > options.MaxNodes {
		return domain.RootPage{}, domain.ErrResultTooLarge
	}
	if hasNext {
		page.Next = &domain.RootCursor{CreatedAt: lastRoot.CreatedAt, ID: lastRoot.ID, Order: options.Order}
	}
	return page, nil
}

func validateRootPage(options domain.RootPageOptions) error {
	if options.Limit < 1 || options.Limit > domain.MaxRootPageSize {
		return fmt.Errorf("root page size must be in 1..%d: %w", domain.MaxRootPageSize, domain.ErrInvalidInput)
	}
	if options.MaxNodes < 1 || options.MaxNodes == math.MaxInt {
		return fmt.Errorf("page node limit must be positive and less than MaxInt: %w", domain.ErrInvalidInput)
	}
	if options.Order != domain.RootASC && options.Order != domain.RootDESC {
		return fmt.Errorf("root order must be asc or desc: %w", domain.ErrInvalidInput)
	}
	if cursor := options.After; cursor != nil {
		if cursor.Order != options.Order || cursor.CreatedAt.IsZero() || cursor.ID <= 0 {
			return fmt.Errorf("root cursor must match the ordering and contain a valid key: %w", domain.ErrInvalidInput)
		}
	}
	return nil
}
