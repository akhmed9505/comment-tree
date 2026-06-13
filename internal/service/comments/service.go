// Package comments provides business logic for working with comments.
package comments

import (
	"context"

	"github.com/akhmed9505/comment-tree/internal/domain"
	repocomments "github.com/akhmed9505/comment-tree/internal/repository/comments"

	"github.com/pkg/errors"
	"github.com/wb-go/wbf/logger"
)

// Service provides comment-related business operations.
type Service struct {
	logger *logger.ZerologAdapter
	repo   Repository
}

// New creates a new comment service.
func New(logger *logger.ZerologAdapter, repo Repository) *Service {
	return &Service{
		logger: logger,
		repo:   repo,
	}
}

// Create stores a new comment and returns its generated ID.
func (s *Service) Create(ctx context.Context, comment *domain.Comment) (int, error) {
	return s.repo.Create(ctx, comment)
}

// Delete removes a comment by ID and maps repository not-found errors to service errors.
func (s *Service) Delete(ctx context.Context, id int) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, repocomments.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

// GetRootComments returns top-level comments with optional search and pagination.
func (s *Service) GetRootComments(ctx context.Context, search *string, limit, offset int) ([]*domain.Comment, error) {
	return s.repo.GetRootComments(ctx, search, limit, offset)
}

// GetChildComments returns all replies for a given parent comment.
func (s *Service) GetChildComments(ctx context.Context, parentID int) ([]*domain.Comment, error) {
	comments, err := s.repo.GetChildComments(ctx, parentID)
	if err != nil {
		if errors.Is(err, repocomments.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return comments, nil
}

// GetAllComments returns all comments as a tree structure.
func (s *Service) GetAllComments(ctx context.Context) ([]domain.CommentNode, error) {
	return s.repo.GetAllComments(ctx)
}
