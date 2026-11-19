package domain

import (
	"testing"
	"time"
)

func TestValidateRootPage(t *testing.T) {
	for _, order := range []RootOrder{RootASC, RootDESC} {
		options := RootPageOptions{Limit: MaxRootPageSize, MaxNodes: 1, Order: order}
		if err := options.Validate(); err != nil {
			t.Fatal(err)
		}
		options.After = &RootCursor{ID: 1, CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 123456000, time.UTC), Order: order}
		if err := options.Validate(); err != nil {
			t.Fatal(err)
		}
	}
}
