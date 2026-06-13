package comments

import (
	"context"

	"github.com/akhmed9505/comment-tree/internal/domain"
)

// Service defines the business logic for working with comments.
//
//go:generate mockgen -source=handlers.go -destination=mocks/mock.go
type Service interface {
	Create(ctx context.Context, comment *domain.Comment) (int, error)
	Delete(ctx context.Context, id int) error
	GetRootComments(ctx context.Context, search *string, limit, offset int) ([]*domain.Comment, error)
	GetChildComments(ctx context.Context, parentID int) ([]*domain.Comment, error)
	GetAllComments(ctx context.Context) ([]domain.CommentNode, error)
}
