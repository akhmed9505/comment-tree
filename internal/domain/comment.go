// Package domain defines comments and their business rules.
package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Comment represents a stored comment. A nil ParentID identifies a root.
type Comment struct {
	ID        int64
	ParentID  *int64
	Author    string
	Body      string
	CreatedAt time.Time
}

// CreateCommentInput contains user-provided data before identity and time are assigned.
type CreateCommentInput struct {
	ParentID *int64
	Author   string
	Body     string
}

// AuthorMaxLength is the maximum author length in Unicode code points.
const AuthorMaxLength = 80

// BodyMaxLength is the maximum body length in Unicode code points.
const BodyMaxLength = 10_000

// NewCommentInput validates creation data and trims surrounding Unicode whitespace.
func NewCommentInput(parentID *int64, author, body string) (CreateCommentInput, error) {
	if parentID != nil {
		if err := ValidateID(*parentID); err != nil {
			return CreateCommentInput{}, fmt.Errorf("parent_id: %w", err)
		}
	}
	author = strings.TrimSpace(author)
	body = strings.TrimSpace(body)
	if err := validateText("author", author, AuthorMaxLength); err != nil {
		return CreateCommentInput{}, err
	}
	if err := validateText("body", body, BodyMaxLength); err != nil {
		return CreateCommentInput{}, err
	}
	input := CreateCommentInput{Author: author, Body: body}
	if parentID != nil {
		id := *parentID
		input.ParentID = &id
	}
	return input, nil
}

// ValidateID rejects nonpositive comment identifiers.
func ValidateID(id int64) error {
	if id <= 0 {
		return fmt.Errorf("%w: comment ID must be positive", ErrInvalidInput)
	}
	return nil
}

func validateText(field, value string, maxLength int) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("%w: %s must be valid UTF-8", ErrInvalidInput, field)
	}
	if strings.ContainsRune(value, 0) {
		return fmt.Errorf("%w: %s must not contain NUL", ErrInvalidInput, field)
	}
	if length := utf8.RuneCountInString(value); length < 1 || length > maxLength {
		return fmt.Errorf("%w: %s length must be in 1..%d characters", ErrInvalidInput, field, maxLength)
	}
	return nil
}
