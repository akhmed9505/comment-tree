// Package comments coordinates comment operations and enforces application limits.
package comments

import (
	"context"
	"errors"
	"math"

	"github.com/akhmed9505/comment-tree/internal/domain"
)

// Repository provides the persistence operations consumed by Service.
type Repository interface {
	Create(context.Context, domain.CreateCommentInput) (domain.Comment, error)
	Delete(context.Context, int64) error
	GetSubtree(context.Context, int64, int) ([]domain.Comment, error)
	ListRoots(context.Context, domain.RootPageOptions) (domain.RootPage, error)
	Search(context.Context, domain.SearchOptions) (domain.SearchPage, error)
}

// DefaultPageSize applies to root and search pages when no size is specified.
const DefaultPageSize = 20

// Service validates comment operations independently of storage and transport.
type Service struct {
	repository Repository
	maxNodes   int
}

// New uses the configured MaxResultNodes limit; the caller owns the repository lifecycle.
func New(repository Repository, maxNodes int) (*Service, error) {
	if repository == nil {
		return nil, errors.New("comment repository is required")
	}
	if maxNodes < 1 || maxNodes == math.MaxInt {
		return nil, errors.New("comment node limit must be positive and less than MaxInt")
	}
	return &Service{repository: repository, maxNodes: maxNodes}, nil
}

// Create validates and normalizes a new comment before storing it.
func (s *Service) Create(ctx context.Context, input domain.CreateCommentInput) (domain.Comment, error) {
	input, err := domain.NewCommentInput(input.ParentID, input.Author, input.Body)
	if err != nil {
		return domain.Comment{}, err
	}
	return s.repository.Create(ctx, input)
}

// Delete removes a comment and its descendants.
func (s *Service) Delete(ctx context.Context, id int64) error {
	if err := domain.ValidateID(id); err != nil {
		return err
	}
	return s.repository.Delete(ctx, id)
}

// GetSubtree retrieves a complete subtree within the configured node limit.
func (s *Service) GetSubtree(ctx context.Context, id int64) ([]domain.Comment, error) {
	if err := domain.ValidateID(id); err != nil {
		return nil, err
	}
	return s.repository.GetSubtree(ctx, id, s.maxNodes)
}

// ListRoots defaults to newest roots and enforces the configured total node limit.
func (s *Service) ListRoots(ctx context.Context, options domain.RootPageOptions) (domain.RootPage, error) {
	if options.Limit == 0 {
		options.Limit = DefaultPageSize
	}
	if options.Order == "" {
		options.Order = domain.RootDESC
	}
	options.MaxNodes = s.maxNodes
	if err := options.Validate(); err != nil {
		return domain.RootPage{}, err
	}
	return s.repository.ListRoots(ctx, options)
}

// Search defaults the page size and caps matches at the configured node limit.
func (s *Service) Search(ctx context.Context, options domain.SearchOptions) (domain.SearchPage, error) {
	if options.Limit == 0 {
		options.Limit = DefaultPageSize
	}
	query, err := options.NormalizedQuery()
	if err != nil {
		return domain.SearchPage{}, err
	}
	options.Query = query
	options.Limit = min(options.Limit, s.maxNodes)
	return s.repository.Search(ctx, options)
}
