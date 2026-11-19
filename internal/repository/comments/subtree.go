package comments

import (
	"context"
	"fmt"
	"math"

	"github.com/akhmed9505/comment-tree/internal/domain"
)

// GetSubtree returns the requested comment first, followed by descendants in chronological order.
// maxNodes includes the requested comment; errors never return a partial result.
func (r *Repository) GetSubtree(ctx context.Context, id int64, maxNodes int) ([]domain.Comment, error) {
	if err := domain.ValidateID(id); err != nil {
		return nil, err
	}
	if maxNodes <= 0 || maxNodes == math.MaxInt {
		return nil, fmt.Errorf("subtree node limit must be positive and less than MaxInt: %w", domain.ErrInvalidInput)
	}
	// Bound the recursive output before sorting, retaining one extra row to detect overflow.
	rows, err := r.pool.Query(ctx, `WITH RECURSIVE subtree AS (
		SELECT id, parent_id, author, body, created_at FROM comments WHERE id = $1
		UNION ALL
		SELECT c.id, c.parent_id, c.author, c.body, c.created_at
		FROM comments c JOIN subtree p ON c.parent_id = p.id
	)
	SELECT id, parent_id, author, body, created_at
	FROM (SELECT * FROM subtree LIMIT $2) bounded
	ORDER BY (id = $1) DESC, created_at ASC, id ASC`, id, maxNodes+1)
	if err != nil {
		return nil, fmt.Errorf("get comment subtree: %w", mapError(ctx, err))
	}
	defer rows.Close()
	var result []domain.Comment
	for rows.Next() {
		var comment domain.Comment
		if err := rows.Scan(&comment.ID, &comment.ParentID, &comment.Author, &comment.Body, &comment.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan comment subtree: %w", mapError(ctx, err))
		}
		result = append(result, comment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read comment subtree: %w", mapError(ctx, err))
	}
	if len(result) > maxNodes {
		return nil, domain.ErrResultTooLarge
	}
	if len(result) == 0 {
		return nil, domain.ErrNotFound
	}
	return result, nil
}
