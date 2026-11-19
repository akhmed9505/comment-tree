// Package comments adapts comment services to JSON HTTP endpoints.
package comments

import (
	"context"
	"errors"
	"io"
	"mime"
	nethttp "net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/akhmed9505/comment-tree/internal/delivery/http/response"
	"github.com/akhmed9505/comment-tree/internal/domain"
)

// Service provides comment operations consumed by the HTTP handlers.
type Service interface {
	Create(context.Context, domain.CreateCommentInput) (domain.Comment, error)
	Delete(context.Context, int64) error
	GetSubtree(context.Context, int64) ([]domain.Comment, error)
	ListRoots(context.Context, domain.RootPageOptions) (domain.RootPage, error)
	Search(context.Context, domain.SearchOptions) (domain.SearchPage, error)
}

// Handler adapts comment operations to HTTP requests.
type Handler struct {
	service Service
}

// New returns comment handlers; middleware supplies request resource limits.
func New(service Service) (*Handler, error) {
	if service == nil {
		return nil, errors.New("HTTP service is required")
	}
	return &Handler{service: service}, nil
}

func parseID(raw string) (int64, error) {
	if len(raw) == 0 || len(raw) > 19 {
		return 0, domain.ErrInvalidInput
	}
	for _, c := range raw {
		if c < '0' || c > '9' {
			return 0, domain.ErrInvalidInput
		}
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, domain.ErrInvalidInput
	}
	return id, nil
}

func parameters(r *nethttp.Request, allowed ...string) (url.Values, error) {
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, domain.ErrInvalidInput
	}
	for key, values := range q {
		ok := false
		for _, name := range allowed {
			if key == name {
				ok = true
			}
		}
		if !ok || len(values) != 1 || values[0] == "" {
			return nil, domain.ErrInvalidInput
		}
	}
	return q, nil
}

func pageLimit(q url.Values) (int, error) {
	if !q.Has("limit") {
		return 0, nil
	}
	v, err := parseID(q.Get("limit"))
	if err != nil || v > 50 {
		return 0, domain.ErrInvalidInput
	}
	return int(v), nil
}

// Create decodes a comment request and returns the created comment.
func (h *Handler) Create(w nethttp.ResponseWriter, r *nethttp.Request) {
	if _, err := parameters(r); err != nil {
		response.Fail(w, r, err)
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		response.Fail(w, r, response.Error{Status: nethttp.StatusUnsupportedMediaType, Code: "unsupported_media_type", Message: "Content-Type must be application/json"})
		return
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var large *nethttp.MaxBytesError
		if errors.As(err, &large) {
			err = response.Error{Status: nethttp.StatusRequestEntityTooLarge, Code: "request_too_large", Message: "Request body exceeds the configured limit"}
		} else {
			err = domain.ErrInvalidInput
		}
		response.Fail(w, r, err)
		return
	}
	var input struct {
		ParentID *string `json:"parent_id"`
		Author   *string `json:"author"`
		Body     *string `json:"body"`
	}
	if decodeObject(data, &input) != nil || input.Author == nil || input.Body == nil {
		response.Fail(w, r, domain.ErrInvalidInput)
		return
	}
	var parent *int64
	if input.ParentID != nil {
		id, err := parseID(*input.ParentID)
		if err != nil {
			response.Fail(w, r, err)
			return
		}
		parent = &id
	}
	c, err := h.service.Create(r.Context(), domain.CreateCommentInput{ParentID: parent, Author: *input.Author, Body: *input.Body})
	if err != nil {
		response.Fail(w, r, err)
		return
	}
	response.WriteJSON(w, nethttp.StatusCreated, toComment(c))
}

// Delete removes the requested comment subtree.
func (h *Handler) Delete(w nethttp.ResponseWriter, r *nethttp.Request) {
	if _, err := parameters(r); err != nil {
		response.Fail(w, r, err)
		return
	}
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		response.Fail(w, r, err)
		return
	}
	if err := h.service.Delete(r.Context(), id); err != nil {
		response.Fail(w, r, err)
		return
	}
	w.WriteHeader(nethttp.StatusNoContent)
}

// List returns root pages or a complete subtree selected by parent.
func (h *Handler) List(w nethttp.ResponseWriter, r *nethttp.Request) {
	q, err := parameters(r, "parent", "limit", "sort", "cursor")
	if err != nil {
		response.Fail(w, r, err)
		return
	}
	if q.Has("parent") {
		if len(q) != 1 {
			response.Fail(w, r, domain.ErrInvalidInput)
			return
		}
		id, err := parseID(q.Get("parent"))
		if err != nil {
			response.Fail(w, r, err)
			return
		}
		result, err := h.service.GetSubtree(r.Context(), id)
		if err != nil {
			response.Fail(w, r, err)
			return
		}
		response.WriteJSON(w, nethttp.StatusOK, commentsResponse{Comments: toComments(result)})
		return
	}
	limit, err := pageLimit(q)
	if err != nil {
		response.Fail(w, r, err)
		return
	}
	options := domain.RootPageOptions{Limit: limit, Order: domain.RootDESC}
	switch q.Get("sort") {
	case "", "newest":
	case "oldest":
		options.Order = domain.RootASC
	default:
		response.Fail(w, r, domain.ErrInvalidInput)
		return
	}
	if q.Has("cursor") {
		c, err := decodeCursor(q.Get("cursor"), "roots")
		if err != nil {
			response.Fail(w, r, err)
			return
		}
		id, _ := parseID(c.ID)
		options.After = &domain.RootCursor{ID: id, CreatedAt: c.CreatedAt, Order: c.Order}
	}
	if options.After != nil && options.After.Order != options.Order {
		response.Fail(w, r, domain.ErrInvalidInput)
		return
	}
	page, err := h.service.ListRoots(r.Context(), options)
	if err != nil {
		response.Fail(w, r, err)
		return
	}
	var next *string
	if c := page.Next; c != nil {
		next, err = encodeCursor(cursorDTO{Kind: "roots", CreatedAt: c.CreatedAt, ID: strconv.FormatInt(c.ID, 10), Order: c.Order})
		if err != nil {
			response.Fail(w, r, err)
			return
		}
	}
	response.WriteJSON(w, nethttp.StatusOK, commentsResponse{Comments: toComments(page.Comments), NextCursor: next})
}

// Search returns matching comments with discussion root IDs.
func (h *Handler) Search(w nethttp.ResponseWriter, r *nethttp.Request) {
	q, err := parameters(r, "q", "limit", "cursor")
	if err != nil {
		response.Fail(w, r, err)
		return
	}
	limit, err := pageLimit(q)
	if err != nil {
		response.Fail(w, r, err)
		return
	}
	options := domain.SearchOptions{Query: q.Get("q"), Limit: limit}
	if q.Has("cursor") {
		c, err := decodeCursor(q.Get("cursor"), "search")
		if err != nil {
			response.Fail(w, r, err)
			return
		}
		id, _ := parseID(c.ID)
		options.After = &domain.SearchCursor{ID: id, CreatedAt: c.CreatedAt, Query: c.Query}
	}
	if !q.Has("q") {
		response.Fail(w, r, domain.ErrInvalidInput)
		return
	}
	page, err := h.service.Search(r.Context(), options)
	if err != nil {
		response.Fail(w, r, err)
		return
	}
	result := searchResponse{Matches: make([]matchDTO, 0, len(page.Matches))}
	for _, m := range page.Matches {
		result.Matches = append(result.Matches, matchDTO{Comment: toComment(m.Comment), RootID: strconv.FormatInt(m.RootID, 10)})
	}
	if c := page.Next; c != nil {
		result.NextCursor, err = encodeCursor(cursorDTO{Kind: "search", CreatedAt: c.CreatedAt, ID: strconv.FormatInt(c.ID, 10), Query: c.Query})
		if err != nil {
			response.Fail(w, r, err)
			return
		}
	}
	response.WriteJSON(w, nethttp.StatusOK, result)
}
