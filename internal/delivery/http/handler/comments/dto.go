package comments

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

type matchDTO struct {
	Comment commentDTO `json:"comment"`
	RootID  string     `json:"root_id"`
}

func toComment(c domain.Comment) commentDTO {
	dto := commentDTO{ID: strconv.FormatInt(c.ID, 10), Author: c.Author, Body: c.Body, CreatedAt: c.CreatedAt}
	if c.ParentID != nil {
		v := strconv.FormatInt(*c.ParentID, 10)
		dto.ParentID = &v
	}
	return dto
}
