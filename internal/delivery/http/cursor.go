package http

import (
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/akhmed9505/comment-tree/internal/domain"
)

type cursorDTO struct {
	Version   int              `json:"version"`
	Kind      string           `json:"kind"`
	CreatedAt time.Time        `json:"created_at"`
	ID        string           `json:"id"`
	Order     domain.RootOrder `json:"order,omitempty"`
	Query     string           `json:"query,omitempty"`
}

func encodeCursor(c cursorDTO) (*string, error) {
	c.Version = 1
	data, err := json.Marshal(c)
	if err != nil {
		return nil, domain.ErrInternal
	}
	encoded := base64.RawURLEncoding.EncodeToString(data)
	return &encoded, nil
}

func decodeCursor(raw, kind string) (cursorDTO, error) {
	var c cursorDTO
	if raw == "" || len(raw) > 4096 {
		return c, domain.ErrInvalidInput
	}
	data, err := base64.RawURLEncoding.Strict().DecodeString(raw)
	if err != nil || base64.RawURLEncoding.EncodeToString(data) != raw || decodeObject(data, &c) != nil || c.Version != 1 || c.Kind != kind || c.CreatedAt.IsZero() {
		return cursorDTO{}, domain.ErrInvalidInput
	}
	if _, err := parseID(c.ID); err != nil {
		return cursorDTO{}, err
	}
	if kind == "roots" && c.Query != "" || kind == "search" && c.Order != "" {
		return cursorDTO{}, domain.ErrInvalidInput
	}
	return c, nil
}
