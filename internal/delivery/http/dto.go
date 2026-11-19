package http

import (
	"strconv"
	"time"

	"github.com/akhmed9505/comment-tree/internal/domain"
)

type commentDTO struct {
	ID        string    `json:"id"`
	ParentID  *string   `json:"parent_id"`
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type commentsResponse struct {
	Comments   []commentDTO `json:"comments"`
	NextCursor *string      `json:"next_cursor"`
}

type matchDTO struct {
	Comment commentDTO `json:"comment"`
	RootID  string     `json:"root_id"`
}

type searchResponse struct {
	Matches    []matchDTO `json:"matches"`
	NextCursor *string    `json:"next_cursor"`
}

func toComment(c domain.Comment) commentDTO {
	dto := commentDTO{ID: strconv.FormatInt(c.ID, 10), Author: c.Author, Body: c.Body, CreatedAt: c.CreatedAt}
	if c.ParentID != nil {
		v := strconv.FormatInt(*c.ParentID, 10)
		dto.ParentID = &v
	}
	return dto
}

func toComments(comments []domain.Comment) []commentDTO {
	result := make([]commentDTO, 0, len(comments))
	for _, c := range comments {
		result = append(result, toComment(c))
	}
	return result
}
