package domain

import "time"

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
