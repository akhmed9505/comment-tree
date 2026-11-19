package comments

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/akhmed9505/comment-tree/internal/domain"
)

func TestListRootsRejectsInvalidOptions(t *testing.T) {
	for name, change := range map[string]func(*domain.RootPageOptions){
		"zero roots":        func(o *domain.RootPageOptions) { o.Limit = 0 },
		"negative roots":    func(o *domain.RootPageOptions) { o.Limit = -1 },
		"too many roots":    func(o *domain.RootPageOptions) { o.Limit = domain.MaxRootPageSize + 1 },
		"zero nodes":        func(o *domain.RootPageOptions) { o.MaxNodes = 0 },
		"negative nodes":    func(o *domain.RootPageOptions) { o.MaxNodes = -1 },
		"overflowing nodes": func(o *domain.RootPageOptions) { o.MaxNodes = math.MaxInt },
		"missing order":     func(o *domain.RootPageOptions) { o.Order = "" },
		"unknown order":     func(o *domain.RootPageOptions) { o.Order = "desc; DROP TABLE comments" },
		"mismatched cursor": func(o *domain.RootPageOptions) {
			o.After = &domain.RootCursor{CreatedAt: time.Now(), ID: 1, Order: domain.RootDESC}
		},
		"missing cursor order": func(o *domain.RootPageOptions) { o.After = &domain.RootCursor{CreatedAt: time.Now(), ID: 1} },
		"missing cursor time":  func(o *domain.RootPageOptions) { o.After = &domain.RootCursor{ID: 1, Order: domain.RootASC} },
		"zero cursor ID": func(o *domain.RootPageOptions) {
			o.After = &domain.RootCursor{CreatedAt: time.Now(), Order: domain.RootASC}
		},
		"negative cursor ID": func(o *domain.RootPageOptions) {
			o.After = &domain.RootCursor{CreatedAt: time.Now(), ID: -1, Order: domain.RootASC}
		},
	} {
		t.Run(name, func(t *testing.T) {
			options := domain.RootPageOptions{Limit: 2, MaxNodes: 100, Order: domain.RootASC}
			change(&options)
			page, err := New(nil).ListRoots(t.Context(), options)
			if !errors.Is(err, domain.ErrInvalidInput) || page.Comments != nil || page.Next != nil {
				t.Fatal("invalid options must fail before querying, without a partial page")
			}
		})
	}
}
