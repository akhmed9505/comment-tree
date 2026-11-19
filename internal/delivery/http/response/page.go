package response

import (
	"context"
	"encoding/json"
	"math"
	"net/http"

	"github.com/akhmed9505/comment-tree/internal/domain"
)

type responseLimiter interface {
	ResponseLimit() int64
}

type pageBuffer struct {
	ctx   context.Context
	limit int64
	data  []byte
}

func (b *pageBuffer) write(data []byte) error {
	if err := b.ctx.Err(); err != nil {
		return err
	}
	if int64(len(data)) > b.limit-int64(len(b.data)) {
		return domain.ErrResultTooLarge
	}
	b.data = append(b.data, data...)
	return nil
}

func (b *pageBuffer) value(value any) error {
	if err := b.ctx.Err(); err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return domain.ErrInternal
	}
	return b.write(data)
}

// WritePage encodes one item at a time within the middleware's response budget.
// Nothing is committed until the entire envelope, including its cursor, fits.
func WritePage[T, D any](w http.ResponseWriter, r *http.Request, field string, items []T, convert func(T) D, next *string) {
	limit := int64(math.MaxInt)
	if bounded, ok := w.(responseLimiter); ok {
		limit = bounded.ResponseLimit()
	}
	buffer := pageBuffer{ctx: r.Context(), limit: limit}
	if err := encodePage(&buffer, field, items, convert, next); err != nil {
		Fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buffer.data)
}

func encodePage[T, D any](b *pageBuffer, field string, items []T, convert func(T) D, next *string) error {
	if err := b.write([]byte{'{'}); err != nil {
		return err
	}
	if err := b.value(field); err != nil {
		return err
	}
	if err := b.write([]byte(`:[`)); err != nil {
		return err
	}
	for i, item := range items {
		if err := b.ctx.Err(); err != nil {
			return err
		}
		if i > 0 {
			if err := b.write([]byte{','}); err != nil {
				return err
			}
		}
		if err := b.value(convert(item)); err != nil {
			return err
		}
	}
	if err := b.write([]byte(`],"next_cursor":`)); err != nil {
		return err
	}
	if err := b.value(next); err != nil {
		return err
	}
	return b.write([]byte("}\n"))
}
