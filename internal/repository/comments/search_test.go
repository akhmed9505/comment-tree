package comments

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/akhmed9505/comment-tree/internal/domain"
)

func TestNormalizeSearch(t *testing.T) {
	for _, query := range []string{"  КОШКИ\t RUNNING\n", "кошки running"} {
		options := domain.SearchOptions{Query: query, Limit: 1, After: &domain.SearchCursor{ID: 1, CreatedAt: time.Now(), Query: "кошки running"}}
		got, err := normalizeSearch(options)
		if err != nil || got != "кошки running" {
			t.Fatal("query normalization must preserve cursor binding")
		}
	}
	if _, err := normalizeSearch(domain.SearchOptions{Query: strings.Repeat("я", domain.MaxSearchQueryLength), Limit: domain.MaxSearchPageSize}); err != nil {
		t.Fatal(err)
	}
}

func TestSearchRejectsInvalidOptions(t *testing.T) {
	for name, change := range map[string]func(*domain.SearchOptions){
		"zero limit":     func(o *domain.SearchOptions) { o.Limit = 0 },
		"negative limit": func(o *domain.SearchOptions) { o.Limit = -1 },
		"large limit":    func(o *domain.SearchOptions) { o.Limit = domain.MaxSearchPageSize + 1 },
		"empty query":    func(o *domain.SearchOptions) { o.Query = "" },
		"blank query":    func(o *domain.SearchOptions) { o.Query = " \t\u00a0" },
		"long query":     func(o *domain.SearchOptions) { o.Query = strings.Repeat("я", domain.MaxSearchQueryLength+1) },
		"invalid UTF8":   func(o *domain.SearchOptions) { o.Query = string([]byte{0xff}) },
		"NUL":            func(o *domain.SearchOptions) { o.Query = "bad\x00query" },
		"other query": func(o *domain.SearchOptions) {
			o.After = &domain.SearchCursor{ID: 1, CreatedAt: time.Now(), Query: "other"}
		},
		"missing time": func(o *domain.SearchOptions) { o.After = &domain.SearchCursor{ID: 1, Query: "cats"} },
		"zero ID":      func(o *domain.SearchOptions) { o.After = &domain.SearchCursor{CreatedAt: time.Now(), Query: "cats"} },
		"negative ID": func(o *domain.SearchOptions) {
			o.After = &domain.SearchCursor{ID: -1, CreatedAt: time.Now(), Query: "cats"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			options := domain.SearchOptions{Query: "cats", Limit: 2}
			change(&options)
			page, err := New(nil).Search(t.Context(), options)
			if !errors.Is(err, domain.ErrInvalidInput) || page.Matches != nil || page.Next != nil {
				t.Fatal("invalid search must fail before querying without partial results")
			}
		})
	}
}
