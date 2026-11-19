// Package response writes JSON responses and safe HTTP errors.
package response

import (
	"context"
	"encoding/json"
	"errors"
	nethttp "net/http"

	"github.com/akhmed9505/comment-tree/internal/domain"
)

// Error describes a public HTTP error.
type Error struct {
	Status        int
	Code, Message string
}

func (e Error) Error() string { return e.Message }

// ErrorResponse is the shared JSON error envelope.
type ErrorResponse struct {
	Error ErrorDTO `json:"error"`
}

// ErrorDTO contains a stable code and safe message.
type ErrorDTO struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Fail maps an error to the shared JSON error response.
func Fail(w nethttp.ResponseWriter, r *nethttp.Request, err error) {
	if errors.Is(err, context.Canceled) || errors.Is(r.Context().Err(), context.Canceled) {
		return
	}
	e := ErrorFor(err)
	if errors.Is(err, domain.ErrLockUnavailable) {
		w.Header().Set("Retry-After", "1")
	}
	WriteJSON(w, e.Status, ErrorResponse{Error: ErrorDTO{Code: e.Code, Message: e.Message}})
}

// ErrorFor classifies errors without exposing diagnostics.
func ErrorFor(err error) Error {
	e := Error{nethttp.StatusInternalServerError, "internal_error", "Internal server error"}
	var specific Error
	switch {
	case errors.As(err, &specific):
		e = specific
	case errors.Is(err, domain.ErrInvalidInput):
		e = Error{nethttp.StatusBadRequest, "invalid_input", "Invalid request"}
	case errors.Is(err, domain.ErrNotFound):
		e = Error{nethttp.StatusNotFound, "comment_not_found", "Comment not found"}
	case errors.Is(err, domain.ErrParentNotFound):
		e = Error{nethttp.StatusNotFound, "parent_not_found", "Parent comment not found"}
	case errors.Is(err, domain.ErrResultTooLarge):
		e = Error{nethttp.StatusRequestEntityTooLarge, "result_too_large", "Result exceeds the configured limit"}
	case errors.Is(err, domain.ErrConflict):
		e = Error{nethttp.StatusConflict, "concurrent_conflict", "Concurrent operation conflict"}
	case errors.Is(err, domain.ErrLockUnavailable):
		e = Error{nethttp.StatusServiceUnavailable, "lock_unavailable", "Required lock is unavailable"}
	case errors.Is(err, domain.ErrUnavailable):
		e = Error{nethttp.StatusServiceUnavailable, "unavailable", "Service temporarily unavailable"}
	case errors.Is(err, context.DeadlineExceeded):
		e = Error{nethttp.StatusGatewayTimeout, "deadline_exceeded", "Request deadline exceeded"}
	case errors.Is(err, domain.ErrQueryInterrupted):
		e = Error{nethttp.StatusGatewayTimeout, "query_interrupted", "Query interrupted"}
	}
	return e
}

// WriteJSON encodes a complete JSON value before committing headers.
func WriteJSON(w nethttp.ResponseWriter, status int, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		status = nethttp.StatusInternalServerError
		data = []byte(`{"error":{"code":"internal_error","message":"Internal server error"}}`)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(append(data, '\n'))
}

// MinimumErrorBytes is the space needed for middleware fallback responses.
func MinimumErrorBytes() int64 {
	var size int64
	for _, err := range []error{domain.ErrInternal, domain.ErrUnavailable, domain.ErrResultTooLarge, context.DeadlineExceeded} {
		e := ErrorFor(err)
		data, _ := json.Marshal(ErrorResponse{Error: ErrorDTO{Code: e.Code, Message: e.Message}})
		size = max(size, int64(len(data)+1))
	}
	return size
}
