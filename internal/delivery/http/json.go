package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	nethttp "net/http"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/akhmed9505/comment-tree/internal/domain"
)

type apiError struct {
	status        int
	code, message string
}

func (e apiError) Error() string { return e.message }

type errorResponse struct {
	Error errorDTO `json:"error"`
}

type errorDTO struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func fail(w nethttp.ResponseWriter, r *nethttp.Request, err error) {
	if errors.Is(err, context.Canceled) || errors.Is(r.Context().Err(), context.Canceled) {
		return
	}
	e := apiError{nethttp.StatusInternalServerError, "internal_error", "Internal server error"}
	var specific apiError
	switch {
	case errors.As(err, &specific):
		e = specific
	case errors.Is(err, domain.ErrInvalidInput):
		e = apiError{nethttp.StatusBadRequest, "invalid_input", "Invalid request"}
	case errors.Is(err, domain.ErrNotFound):
		e = apiError{nethttp.StatusNotFound, "comment_not_found", "Comment not found"}
	case errors.Is(err, domain.ErrParentNotFound):
		e = apiError{nethttp.StatusNotFound, "parent_not_found", "Parent comment not found"}
	case errors.Is(err, domain.ErrResultTooLarge):
		e = apiError{nethttp.StatusRequestEntityTooLarge, "result_too_large", "Result exceeds the configured limit"}
	case errors.Is(err, domain.ErrConflict):
		e = apiError{nethttp.StatusConflict, "concurrent_conflict", "Concurrent operation conflict"}
	case errors.Is(err, domain.ErrLockUnavailable):
		e = apiError{nethttp.StatusServiceUnavailable, "lock_unavailable", "Required lock is unavailable"}
		w.Header().Set("Retry-After", "1")
	case errors.Is(err, domain.ErrUnavailable):
		e = apiError{nethttp.StatusServiceUnavailable, "unavailable", "Service temporarily unavailable"}
	case errors.Is(err, context.DeadlineExceeded):
		e = apiError{nethttp.StatusGatewayTimeout, "deadline_exceeded", "Request deadline exceeded"}
	case errors.Is(err, domain.ErrQueryInterrupted):
		e = apiError{nethttp.StatusGatewayTimeout, "query_interrupted", "Query interrupted"}
	}
	writeJSON(w, e.status, errorResponse{Error: errorDTO{Code: e.code, Message: e.message}})
}

func writeJSON(w nethttp.ResponseWriter, status int, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		status = nethttp.StatusInternalServerError
		data = []byte(`{"error":{"code":"internal_error","message":"Internal server error"}}`)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(append(data, '\n'))
}

// decodeObject rejects duplicate keys, unknown fields and trailing JSON values.

func decodeObject(data []byte, value any) error {
	if !utf8.Valid(data) {
		return domain.ErrInvalidInput
	}
	d := json.NewDecoder(bytes.NewReader(data))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return domain.ErrInvalidInput
	}
	allowed := map[string]bool{}
	typ := reflect.TypeOf(value).Elem()
	for i := 0; i < typ.NumField(); i++ {
		name := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		if name != "" && name != "-" {
			allowed[name] = true
		}
	}
	seen := map[string]bool{}
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return domain.ErrInvalidInput
		}
		key, ok := token.(string)
		if !ok || !allowed[key] || seen[key] {
			return domain.ErrInvalidInput
		}
		seen[key] = true
		var raw json.RawMessage
		if d.Decode(&raw) != nil {
			return domain.ErrInvalidInput
		}
	}
	if _, err := d.Token(); err != nil {
		return domain.ErrInvalidInput
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return domain.ErrInvalidInput
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return domain.ErrInvalidInput
	}
	return nil
}
