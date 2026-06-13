package comments

import (
	"time"
)

// createRequest defines the input payload required to persist a new comment.
type createRequest struct {
	// Content is the raw text body of the comment.
	Content string `json:"content"`
	// ParentID is a pointer to the parent comment's identifier.
	// A nil value indicates a top-level (root) comment.
	ParentID *int `json:"parent_id"`
}

// comment represents the core data structure of a comment resource
// with its associated metadata.
type comment struct {
	ID        int       `json:"id"`
	Content   string    `json:"content,omitempty"`
	ParentID  *int      `json:"parent_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Response facilitates a hierarchical tree representation of comments,
// allowing for nested replies in the API output.
type Response struct {
	comment
	// Children holds a recursive slice of nested replies.
	// The omitempty tag ensures a clean response for leaf nodes.
	Children []*Response `json:"children,omitempty"`
}
