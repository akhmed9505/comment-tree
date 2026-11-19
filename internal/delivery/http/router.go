// Package http assembles HTTP routes and server settings.
package http

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/akhmed9505/comment-tree/internal/delivery/http/handler/comments"
	"github.com/akhmed9505/comment-tree/internal/delivery/http/handler/health"
	"github.com/akhmed9505/comment-tree/internal/delivery/http/response"
)

// NewRouter registers API and health endpoints without owning their dependencies.
func NewRouter(comments *comments.Handler, health *health.Handler) http.Handler {
	r := chi.NewRouter()
	r.Post("/comments", comments.Create)
	r.Get("/comments", comments.List)
	r.Get("/comments/search", comments.Search)
	r.Delete("/comments/{id}", comments.Delete)
	r.Get("/healthz", health.Healthz)
	r.Get("/readyz", health.Readyz)
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		response.Fail(w, r, response.Error{Status: http.StatusNotFound, Code: "not_found", Message: "Resource not found"})
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, request *http.Request) {
		var allowed []string
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
			if r.Match(chi.NewRouteContext(), method, request.URL.Path) {
				allowed = append(allowed, method)
			}
		}
		w.Header().Set("Allow", strings.Join(allowed, ", "))
		response.Fail(w, request, response.Error{Status: http.StatusMethodNotAllowed, Code: "method_not_allowed", Message: "Method not allowed"})
	})
	return r
}
