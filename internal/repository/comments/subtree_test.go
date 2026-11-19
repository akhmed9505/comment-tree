package comments

import (
	"errors"
	"math"
	"testing"

	"github.com/akhmed9505/comment-tree/internal/domain"
)

func TestGetSubtreeRejectsInvalidArguments(t *testing.T) {
	repo := New(nil)
	for _, tc := range []struct {
		name  string
		id    int64
		limit int
	}{
		{"zero ID", 0, 1}, {"negative ID", -1, 1},
		{"zero limit", 1, 0}, {"negative limit", 1, -1}, {"overflowing limit", 1, math.MaxInt},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := repo.GetSubtree(t.Context(), tc.id, tc.limit)
			if !errors.Is(err, domain.ErrInvalidInput) || got != nil {
				t.Fatal("invalid arguments must fail before querying")
			}
		})
	}
}
