package domain

import "errors"

// ErrInvalidInput indicates a violation of comment input rules.
var ErrInvalidInput = errors.New("invalid input")

// ErrNotFound indicates that the requested comment does not exist.
var ErrNotFound = errors.New("comment not found")

// ErrParentNotFound indicates that the parent of a new reply does not exist.
var ErrParentNotFound = errors.New("parent comment not found")
