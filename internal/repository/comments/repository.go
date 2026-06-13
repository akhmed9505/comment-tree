// Package comments provides access to comment persistence operations.
package comments

import (
	"context"
	"fmt"

	"github.com/akhmed9505/comment-tree/internal/domain"

	pgxdriver "github.com/wb-go/wbf/dbpg/pgx-driver"
)

// Repository provides comment storage operations backed by PostgreSQL.
type Repository struct {
	pool *pgxdriver.Postgres
}

// New creates a new comment repository.
func New(pool *pgxdriver.Postgres) *Repository {
	return &Repository{pool: pool}
}

// Create inserts a new comment and returns its generated ID.
func (r *Repository) Create(ctx context.Context, comm *domain.Comment) (int, error) {
	const query = `INSERT INTO comments (content, parent_id)
		VALUES ($1, $2) RETURNING id`

	var id int
	if err := r.pool.QueryRow(ctx, query, comm.Content, comm.ParentID).Scan(&id); err != nil {
		return 0, fmt.Errorf("failed to create comment: %w", err)
	}

	return id, nil
}

// Delete removes a comment by ID.
func (r *Repository) Delete(ctx context.Context, id int) error {
	const query = `DELETE FROM comments WHERE id = $1`

	res, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete comment: %w", err)
	}

	if res.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// GetRootComments returns root comments with optional search, pagination support.
func (r *Repository) GetRootComments(ctx context.Context, search *string, limit, offset int) ([]*domain.Comment, error) {
	const query = `
		SELECT id, content, parent_id, created_at, updated_at
		FROM comments
		WHERE parent_id IS NULL
		  AND ($1::text IS NULL OR to_tsvector('russian', content) @@ plainto_tsquery('russian', $1::text))
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	var searchVal any
	if search != nil {
		searchVal = *search
	}

	rows, err := r.pool.Query(ctx, query, searchVal, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get root comments: %w", err)
	}
	defer rows.Close()

	var comments []*domain.Comment
	for rows.Next() {
		var c domain.Comment
		if err := rows.Scan(&c.ID, &c.Content, &c.ParentID, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan root comments: %w", err)
		}
		comments = append(comments, &c)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return comments, nil
}

// GetChildComments returns all child comments for the specified parent comment.
func (r *Repository) GetChildComments(ctx context.Context, parentID int) ([]*domain.Comment, error) {
	const query = `
		SELECT id, content, parent_id, created_at, updated_at
		FROM comments
		WHERE parent_id = $1
		ORDER BY created_at ASC
	`

	rows, err := r.pool.Query(ctx, query, parentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get child comments: %w", err)
	}
	defer rows.Close()

	var comments []*domain.Comment
	for rows.Next() {
		var c domain.Comment
		if err := rows.Scan(&c.ID, &c.Content, &c.ParentID, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan child comments: %w", err)
		}
		comments = append(comments, &c)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return comments, nil
}

// GetAllComments returns all comments as a tree structure.
func (r *Repository) GetAllComments(ctx context.Context) ([]domain.CommentNode, error) {
	const query = `
		SELECT id, content, parent_id, created_at, updated_at
		FROM comments
		ORDER BY created_at DESC
	`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get comments: %w", err)
	}
	defer rows.Close()

	var comments []domain.Comment
	for rows.Next() {
		var c domain.Comment
		if err := rows.Scan(&c.ID, &c.Content, &c.ParentID, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan comments: %w", err)
		}
		comments = append(comments, c)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	nodeMap := make(map[int]*domain.CommentNode, len(comments))
	roots := make([]domain.CommentNode, 0)

	for i := range comments {
		comment := comments[i]
		node := &domain.CommentNode{
			Comment:  comment,
			Children: []domain.CommentNode{},
		}
		nodeMap[comment.ID] = node
	}

	for _, node := range nodeMap {
		if node.Comment.ParentID == nil {
			roots = append(roots, *node)
			continue
		}

		if parentNode, ok := nodeMap[*node.Comment.ParentID]; ok {
			parentNode.Children = append(parentNode.Children, *node)
		} else {
			roots = append(roots, *node)
		}
	}

	return roots, nil
}
