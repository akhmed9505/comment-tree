package domain

import (
	"fmt"
	"math"
	"time"
)

// RootOrder determines the order of root comments only.
type RootOrder string

// RootASC and RootDESC order roots by their creation timestamp and ID.
const (
	RootASC  RootOrder = "asc"
	RootDESC RootOrder = "desc"
)

// MaxRootPageSize bounds the number of roots selected per page.
const MaxRootPageSize = 50

// RootCursor marks a position in a particular root ordering.
type RootCursor struct {
	CreatedAt time.Time
	ID        int64
	Order     RootOrder
}

// RootPageOptions limits roots and total nodes independently.
type RootPageOptions struct {
	Limit    int
	MaxNodes int
	Order    RootOrder
	After    *RootCursor
}

// RootPage contains complete subtrees and an optional continuation boundary.
type RootPage struct {
	Comments []Comment
	Next     *RootCursor
}

// Validate checks page bounds and cursor ordering.
func (options RootPageOptions) Validate() error {
	if options.Limit < 1 || options.Limit > MaxRootPageSize {
		return fmt.Errorf("root page size must be in 1..%d: %w", MaxRootPageSize, ErrInvalidInput)
	}
	if options.MaxNodes < 1 || options.MaxNodes == math.MaxInt {
		return fmt.Errorf("page node limit must be positive and less than MaxInt: %w", ErrInvalidInput)
	}
	if options.Order != RootASC && options.Order != RootDESC {
		return fmt.Errorf("root order must be asc or desc: %w", ErrInvalidInput)
	}
	if cursor := options.After; cursor != nil {
		if cursor.Order != options.Order || cursor.CreatedAt.IsZero() || cursor.ID <= 0 {
			return fmt.Errorf("root cursor must match the ordering and contain a valid key: %w", ErrInvalidInput)
		}
	}
	return nil
}
