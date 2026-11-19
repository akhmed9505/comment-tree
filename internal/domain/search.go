package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// MaxSearchPageSize bounds the number of matches returned per page.
const MaxSearchPageSize = 50

// MaxSearchQueryLength bounds search input in Unicode code points.
const MaxSearchQueryLength = 256

// SearchCursor marks a descending search boundary bound to a normalized query.
type SearchCursor struct {
	CreatedAt time.Time
	ID        int64
	Query     string
}

// SearchOptions configures a page of matching comments.
type SearchOptions struct {
	Query string
	Limit int
	After *SearchCursor
}

// SearchMatch identifies a matching comment and the root of its discussion.
type SearchMatch struct {
	Comment Comment
	RootID  int64
}

// SearchPage contains matches without their subtrees and an optional continuation boundary.
type SearchPage struct {
	Matches []SearchMatch
	Next    *SearchCursor
}

// NormalizedQuery validates search options and returns normalized query text.
func (options SearchOptions) NormalizedQuery() (string, error) {
	if options.Limit < 1 || options.Limit > MaxSearchPageSize {
		return "", fmt.Errorf("search page size must be in 1..%d: %w", MaxSearchPageSize, ErrInvalidInput)
	}
	if !utf8.ValidString(options.Query) || strings.ContainsRune(options.Query, 0) {
		return "", fmt.Errorf("search query must be valid UTF-8 without NUL: %w", ErrInvalidInput)
	}
	query := strings.ToLower(strings.Join(strings.Fields(options.Query), " "))
	if query == "" || utf8.RuneCountInString(query) > MaxSearchQueryLength {
		return "", fmt.Errorf("search query length must be in 1..%d characters: %w", MaxSearchQueryLength, ErrInvalidInput)
	}
	if cursor := options.After; cursor != nil {
		if cursor.ID <= 0 || cursor.CreatedAt.IsZero() || cursor.Query != query {
			return "", fmt.Errorf("search cursor must contain a valid key and match the normalized query: %w", ErrInvalidInput)
		}
	}
	return query, nil
}
