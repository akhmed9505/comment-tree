// Package domain defines the core domain models used by the application.
package domain

import "time"

// Comment represents a comment in the domain model.
type Comment struct {
	ID        int
	Content   string
	ParentID  *int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CommentNode represents a comment with nested child comments.
type CommentNode struct {
	Comment  Comment
	Children []CommentNode
}
