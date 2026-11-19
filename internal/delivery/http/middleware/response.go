package middleware

import (
	"context"
	nethttp "net/http"

	"github.com/akhmed9505/comment-tree/internal/domain"
)

// boundedResponse is confined to the handler goroutine until that goroutine finishes.
type boundedResponse struct {
	header nethttp.Header
	body   []byte
	status int
	limit  int64
	ctx    context.Context
	cancel context.CancelCauseFunc
}

func (w *boundedResponse) Header() nethttp.Header { return w.header }

func (w *boundedResponse) WriteHeader(status int) {
	if status < 100 || status > 999 {
		panic("invalid HTTP status")
	}
	if w.status != 0 || status < 200 {
		return
	}
	w.status = status
}

func (w *boundedResponse) Write(data []byte) (int, error) {
	if w.status == nethttp.StatusNoContent || w.status == nethttp.StatusNotModified {
		return 0, nethttp.ErrBodyNotAllowed
	}
	if w.ctx.Err() != nil {
		return 0, context.Cause(w.ctx)
	}
	if int64(len(data)) > w.limit-int64(len(w.body)) {
		w.body = nil
		w.cancel(domain.ErrResultTooLarge)
		return 0, domain.ErrResultTooLarge
	}
	if w.status == 0 {
		w.status = nethttp.StatusOK
	}
	w.body = append(w.body, data...)
	return len(data), nil
}
