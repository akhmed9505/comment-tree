package domain

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func TestNewCommentInput(t *testing.T) {
	parentID := int64(42)
	input, err := NewCommentInput(&parentID, "\u2003 Alice \t", "\n Привет!\n  Hello! \u00a0")
	if err != nil {
		t.Fatal(err)
	}
	if input.Author != "Alice" || input.Body != "Привет!\n  Hello!" {
		t.Fatalf("unexpected normalization: %#v", input)
	}
	parentID = 99
	if input.ParentID == nil || *input.ParentID != 42 {
		t.Fatal("parent ID must be copied from the caller")
	}
	root, err := NewCommentInput(nil, "Alice", "Hello")
	if err != nil || root.ParentID != nil {
		t.Fatalf("expected a root comment, got %v", err)
	}
}

func TestCommentTextBoundaries(t *testing.T) {
	for _, symbol := range []string{"a", "я", "😀"} {
		t.Run(symbol, func(t *testing.T) {
			author := strings.Repeat(symbol, AuthorMaxLength)
			body := strings.Repeat(symbol, BodyMaxLength)
			if _, err := NewCommentInput(nil, author, body); err != nil {
				t.Fatalf("maximum-length Unicode text was rejected: %v", err)
			}
			if _, err := NewCommentInput(nil, author+symbol, body); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("overlong author: %v", err)
			}
			if _, err := NewCommentInput(nil, author, body+symbol); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("overlong body: %v", err)
			}
		})
	}
}

func TestInvalidCommentInput(t *testing.T) {
	for _, tc := range []struct {
		name   string
		author string
		body   string
		field  string
	}{
		{"empty author", "", "body", "author"},
		{"blank author", " \t\u2003", "body", "author"},
		{"empty body", "author", "", "body"},
		{"blank body", "author", "\n\u00a0", "body"},
		{"author encoding", "private\xff", "body", "author"},
		{"body encoding", "author", "private\xff", "body"},
		{"author NUL", "private\x00", "body", "author"},
		{"body NUL", "author", "private\x00", "body"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input, err := NewCommentInput(nil, tc.author, tc.body)
			if !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), tc.field) {
				t.Fatalf("expected a field validation error, got %v", err)
			}
			if input != (CreateCommentInput{}) {
				t.Fatal("invalid input must not return partially normalized data")
			}
			if strings.Contains(err.Error(), "private") {
				t.Fatal("validation error exposed comment text")
			}
		})
	}
}

func TestCommentIdentifiers(t *testing.T) {
	for _, id := range []int64{math.MinInt64, -1, 0, 1, math.MaxInt64} {
		err := ValidateID(id)
		_, parentErr := NewCommentInput(&id, "author", "body")
		if id > 0 {
			if err != nil || parentErr != nil {
				t.Fatalf("positive ID rejected: %v, %v", err, parentErr)
			}
			continue
		}
		if !errors.Is(err, ErrInvalidInput) || !errors.Is(parentErr, ErrInvalidInput) ||
			!strings.Contains(parentErr.Error(), "parent_id") {
			t.Fatalf("expected invalid ID errors: %v, %v", err, parentErr)
		}
	}
}
