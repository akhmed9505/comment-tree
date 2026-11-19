package domain

import "time"

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
