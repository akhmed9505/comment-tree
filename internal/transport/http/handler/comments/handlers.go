// Package comments provides HTTP handlers for creating, reading, and deleting comments.
package comments

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/akhmed9505/comment-tree/internal/config"
	"github.com/akhmed9505/comment-tree/internal/domain"
	svccomments "github.com/akhmed9505/comment-tree/internal/service/comments"
	"github.com/akhmed9505/comment-tree/internal/transport/http/helpers"
	"github.com/akhmed9505/comment-tree/internal/transport/http/response"

	"github.com/wb-go/wbf/ginext"
	"github.com/wb-go/wbf/logger"
)

// Handler implements HTTP transport logic for comment-related operations.
type Handler struct {
	srv Service
	cfg *config.Config
	log *logger.ZerologAdapter
}

// New initializes and returns a new Handler instance with required dependencies.
func New(srv Service, cfg *config.Config, log *logger.ZerologAdapter) *Handler {
	return &Handler{
		srv: srv,
		cfg: cfg,
		log: log,
	}
}

// Create handles the ingestion of a new comment.
func (h *Handler) Create(c *ginext.Context) {
	var req createRequest

	if err := helpers.DecodeJSON(c.Request, &req); err != nil {
		h.log.Warn("request decoding failed",
			"err", err,
			"component", "comments_handler",
			"method", "Create")
		_ = response.Fail(c.Writer, http.StatusBadRequest, ErrBadRequest)
		return
	}

	comment := &domain.Comment{
		Content:  req.Content,
		ParentID: req.ParentID,
	}

	id, err := h.srv.Create(c.Request.Context(), comment)
	if err != nil {
		h.log.Error("comment persistence failed",
			"err", err,
			"parent_id", req.ParentID,
			"component", "comments_handler")
		_ = response.Fail(c.Writer, http.StatusInternalServerError, ErrInternalServerError)
		return
	}

	// Success log for audit trails
	h.log.Info("comment created successfully", "id", id, "parent_id", req.ParentID)
	_ = response.Created(c.Writer, id)
}

// Delete removes a specific comment resource by its identifier.
func (h *Handler) Delete(c *ginext.Context) {
	id, err := helpers.ParseIntParam(c, "id")
	if err != nil {
		h.log.Warn("invalid id parameter in delete request", "err", err)
		_ = response.Fail(c.Writer, http.StatusBadRequest, ErrBadRequest)
		return
	}

	if err := h.srv.Delete(c.Request.Context(), id); err != nil {
		if errors.Is(err, svccomments.ErrNotFound) {
			h.log.Warn("attempt to delete non-existent comment", "id", id)
			_ = response.Fail(c.Writer, http.StatusNotFound, ErrNotFound)
			return
		}

		h.log.Error("failed to delete comment", "id", id, "err", err)
		_ = response.Fail(c.Writer, http.StatusInternalServerError, ErrInternalServerError)
		return
	}

	// Logging successful deletion is a good practice for debugging state changes
	h.log.Info("comment deleted successfully", "id", id)
	c.Status(http.StatusNoContent)
}

// GetRootComments retrieves a paginated list of top-level comments.
func (h *Handler) GetRootComments(c *ginext.Context) {
	limitStr := c.DefaultQuery("limit", "10")
	offsetStr := c.DefaultQuery("offset", "0")
	search := c.Query("search")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 10
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}

	var searchPtr *string
	if search != "" {
		searchPtr = &search
	}

	comments, err := h.srv.GetRootComments(c.Request.Context(), searchPtr, limit, offset)
	if err != nil {
		h.log.Error("failed to fetch root comments",
			"err", err,
			"limit", limit,
			"offset", offset,
			"search", search)
		_ = response.Fail(c.Writer, http.StatusInternalServerError, ErrInternalServerError)
		return
	}

	_ = response.OK(c.Writer, comments)
}

// GetChildComments returns a flattened list of replies for the specified parent comment ID.
func (h *Handler) GetChildComments(c *ginext.Context) {
	parentID, err := helpers.ParseIntParam(c, "parent_id")
	if err != nil {
		h.log.Warn("invalid parent_id parameter", "err", err)
		_ = response.Fail(c.Writer, http.StatusBadRequest, ErrBadRequest)
		return
	}

	comments, err := h.srv.GetChildComments(c.Request.Context(), parentID)
	if err != nil {
		h.log.Error("failed to fetch child comments",
			"parent_id", parentID,
			"err", err)
		_ = response.Fail(c.Writer, http.StatusInternalServerError, ErrInternalServerError)
		return
	}

	_ = response.OK(c.Writer, comments)
}

// GetCommentTree fetches the entire comment set as a tree structure.
func (h *Handler) GetCommentTree(c *ginext.Context) {
	tree, err := h.srv.GetAllComments(c.Request.Context())
	if err != nil {
		h.log.Error("failed to reconstruct comment tree", "err", err)
		_ = response.Fail(c.Writer, http.StatusInternalServerError, ErrInternalServerError)
		return
	}

	_ = response.OK(c.Writer, tree)
}
