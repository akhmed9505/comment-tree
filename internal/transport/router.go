// Package transport provides HTTP routing and middleware setup for the application.
package transport

import (
	"net/http"

	"github.com/akhmed9505/comment-tree/internal/transport/http/handler/comments"

	"github.com/gin-contrib/cors"
	"github.com/wb-go/wbf/ginext"
)

// Handlers groups all HTTP handlers used by the router.
type Handlers struct {
	Comments *comments.Handler
}

// NewRouter creates and configures the HTTP router with middleware and routes.
func NewRouter(handlers Handlers) http.Handler {
	r := ginext.New("")

	config := cors.DefaultConfig()
	config.AllowOrigins = []string{"http://localhost:3000"}
	config.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD"}
	config.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "Authorization"}
	config.AllowCredentials = true

	r.Use(cors.New(config))

	r.Use(ginext.Logger())
	r.Use(ginext.Recovery())

	r.POST("/comments", handlers.Comments.Create)
	r.GET("/comments", handlers.Comments.GetRootComments)
	r.GET("/comments/all", handlers.Comments.GetCommentTree)
	r.GET("/comments/:parent_id/children", handlers.Comments.GetChildComments)
	r.DELETE("/comments/:id", handlers.Comments.Delete)

	return r
}
