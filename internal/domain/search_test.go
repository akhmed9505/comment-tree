package domain

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeSearch(t *testing.T) {
	for _, query := range []string{"  КОШКИ\t RUNNING\n", "кошки running"} {
		options := SearchOptions{Query: query, Limit: 1, After: &SearchCursor{ID: 1, CreatedAt: time.Now(), Query: "кошки running"}}
		got, err := options.NormalizedQuery()
		if err != nil || got != "кошки running" {
			t.Fatal("query normalization must preserve cursor binding")
		}
	}
	if _, err := (SearchOptions{Query: strings.Repeat("я", MaxSearchQueryLength), Limit: MaxSearchPageSize}).NormalizedQuery(); err != nil {
		t.Fatal(err)
	}
}
