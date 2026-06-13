// Package helpers provides utility functions for decoding JSON requests
// and parsing HTTP path parameters.
package helpers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/wb-go/wbf/ginext"
)

// ErrMissingID indicates that a required URL parameter is missing.
var ErrMissingID = errors.New("id is missing")

// ErrInvalidID indicates that a URL parameter has an invalid format.
var ErrInvalidID = errors.New("id has invalid format")

// DecodeJSON decodes JSON from the request body into dst.
// It disallows unknown fields to catch client errors early.
func DecodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

// ParseIntParam extracts and parses an int from a Gin URL parameter.
// It returns ErrMissingID if the parameter is missing and ErrInvalidID if it cannot be parsed.
func ParseIntParam(c *ginext.Context, param string) (int, error) {
	idStr := c.Param(param)
	if idStr == "" {
		return 0, ErrMissingID
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		return 0, ErrInvalidID
	}

	return id, nil
}
