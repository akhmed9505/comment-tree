// Package comments stores comments in PostgreSQL.
package comments

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/akhmed9505/comment-tree/internal/domain"
)

// Repository stores comments using a shared connection pool.
type Repository struct {
	pool *pgxpool.Pool
}

// New returns a repository; the caller retains ownership of pool.
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Create stores a comment and returns its database-generated identity and timestamp.
func (r *Repository) Create(ctx context.Context, input domain.CreateCommentInput) (domain.Comment, error) {
	input, err := domain.NewCommentInput(input.ParentID, input.Author, input.Body)
	if err != nil {
		return domain.Comment{}, err
	}
	var comment domain.Comment
	err = r.pool.QueryRow(ctx, `INSERT INTO comments (parent_id, author, body)
		VALUES ($1, $2, $3) RETURNING id, parent_id, author, body, created_at`,
		input.ParentID, input.Author, input.Body).Scan(
		&comment.ID, &comment.ParentID, &comment.Author, &comment.Body, &comment.CreatedAt)
	if err != nil {
		return domain.Comment{}, fmt.Errorf("create comment: %w", mapCreateError(ctx, err))
	}
	return comment, nil
}

// Delete removes a comment and its descendants atomically.
func (r *Repository) Delete(ctx context.Context, id int64) error {
	if err := domain.ValidateID(id); err != nil {
		return err
	}
	result, err := r.pool.Exec(ctx, "DELETE FROM comments WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete comment: %w", mapError(ctx, err))
	}
	if result.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}
