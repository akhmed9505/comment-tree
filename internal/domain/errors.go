package domain

import "errors"

// ErrInvalidInput indicates a violation of comment input rules.
var ErrInvalidInput = errors.New("invalid input")

// ErrNotFound indicates that the requested comment does not exist.
var ErrNotFound = errors.New("comment not found")

// ErrParentNotFound indicates that the parent of a new reply does not exist.
var ErrParentNotFound = errors.New("parent comment not found")

// ErrConflict indicates a concurrent operation that could not complete.
var ErrConflict = errors.New("concurrent conflict")

// ErrUnavailable indicates a temporarily unavailable dependency.
var ErrUnavailable = errors.New("dependency unavailable")

// ErrLockUnavailable indicates that a required database lock could not be acquired.
var ErrLockUnavailable = errors.New("lock unavailable")

// ErrQueryInterrupted indicates server-side query cancellation with a live caller context.
var ErrQueryInterrupted = errors.New("query interrupted")

// ErrInternal indicates an unexpected failure without exposing infrastructure diagnostics.
var ErrInternal = errors.New("internal error")

// ErrResultTooLarge indicates that a complete result exceeds a configured resource limit.
var ErrResultTooLarge = errors.New("result too large")
