package comments

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/akhmed9505/comment-tree/internal/domain"
)

type repositoryStub struct {
	create  func(context.Context, domain.CreateCommentInput) (domain.Comment, error)
	delete  func(context.Context, int64) error
	subtree func(context.Context, int64, int) ([]domain.Comment, error)
	roots   func(context.Context, domain.RootPageOptions) (domain.RootPage, error)
	search  func(context.Context, domain.SearchOptions) (domain.SearchPage, error)
}

func (r repositoryStub) Create(ctx context.Context, in domain.CreateCommentInput) (domain.Comment, error) {
	return r.create(ctx, in)
}
func (r repositoryStub) Delete(ctx context.Context, id int64) error { return r.delete(ctx, id) }
func (r repositoryStub) GetSubtree(ctx context.Context, id int64, n int) ([]domain.Comment, error) {
	return r.subtree(ctx, id, n)
}
func (r repositoryStub) ListRoots(ctx context.Context, o domain.RootPageOptions) (domain.RootPage, error) {
	return r.roots(ctx, o)
}
func (r repositoryStub) Search(ctx context.Context, o domain.SearchOptions) (domain.SearchPage, error) {
	return r.search(ctx, o)
}

func newService(t *testing.T, r Repository, maxNodes int) *Service {
	t.Helper()
	s, err := New(r, maxNodes)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestNew(t *testing.T) {
	if s, err := New(nil, 100); err == nil || s != nil {
		t.Fatal("nil repository must fail")
	}
	for _, limit := range []int{0, -1, math.MaxInt} {
		if s, err := New(repositoryStub{}, limit); err == nil || s != nil {
			t.Fatal("invalid node limit must fail")
		}
	}
	newService(t, repositoryStub{}, 1)
}

func TestForwarding(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	parent := int64(1)
	want := domain.Comment{ID: 2, ParentID: &parent, Author: "Автор", Body: "Hello\nWorld", CreatedAt: time.Now()}
	rootPage := domain.RootPage{Comments: []domain.Comment{want}}
	searchPage := domain.SearchPage{Matches: []domain.SearchMatch{{Comment: want, RootID: parent}}}
	called := make(map[string]bool)
	check := func(got context.Context, name string) {
		t.Helper()
		if got != ctx {
			t.Fatal("context was replaced")
		}
		called[name] = true
	}
	r := repositoryStub{
		create: func(c context.Context, in domain.CreateCommentInput) (domain.Comment, error) {
			check(c, "create")
			if in.Author != want.Author || in.Body != want.Body || in.ParentID == nil || *in.ParentID != parent || in.ParentID == &parent {
				t.Fatal("create input was not normalized and copied")
			}
			return want, nil
		},
		delete: func(c context.Context, id int64) error {
			check(c, "delete")
			if id != 2 {
				t.Fatal("wrong ID")
			}
			return nil
		},
		subtree: func(c context.Context, id int64, n int) ([]domain.Comment, error) {
			check(c, "subtree")
			if id != 2 || n != 123 {
				t.Fatal("configured node limit was lost")
			}
			return []domain.Comment{want}, nil
		},
		roots: func(c context.Context, o domain.RootPageOptions) (domain.RootPage, error) {
			check(c, "roots")
			if o.Limit != 20 || o.Order != domain.RootDESC || o.MaxNodes != 123 || o.After != nil {
				t.Fatal("root defaults or configured node limit were lost")
			}
			return rootPage, nil
		},
		search: func(c context.Context, o domain.SearchOptions) (domain.SearchPage, error) {
			check(c, "search")
			if o.Query != "кошки run" || o.Limit != 20 {
				t.Fatal("search normalization or defaults were lost")
			}
			return searchPage, nil
		},
	}
	s := newService(t, r, 123)
	got, err := s.Create(ctx, domain.CreateCommentInput{ParentID: &parent, Author: "  Автор\u00a0", Body: "\n Hello\nWorld \t"})
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("create result changed")
	}
	if err := s.Delete(ctx, 2); err != nil {
		t.Fatal(err)
	}
	tree, err := s.GetSubtree(ctx, 2)
	if err != nil || !reflect.DeepEqual(tree, []domain.Comment{want}) {
		t.Fatal("subtree result changed")
	}
	roots, err := s.ListRoots(ctx, domain.RootPageOptions{MaxNodes: math.MaxInt})
	if err != nil || !reflect.DeepEqual(roots, rootPage) {
		t.Fatal("root result changed")
	}
	search, err := s.Search(ctx, domain.SearchOptions{Query: "  КОШКИ\tRUN "})
	if err != nil || !reflect.DeepEqual(search, searchPage) {
		t.Fatal("search result changed")
	}
	if len(called) != 5 {
		t.Fatal("repository method was not called")
	}
}

func TestExplicitPaginationAndLimits(t *testing.T) {
	cursor := &domain.RootCursor{ID: 7, CreatedAt: time.Now(), Order: domain.RootASC}
	searchCursor := &domain.SearchCursor{ID: 8, CreatedAt: time.Now(), Query: "cats"}
	r := repositoryStub{
		roots: func(_ context.Context, o domain.RootPageOptions) (domain.RootPage, error) {
			if o.Limit != 5 || o.Order != domain.RootASC || o.After != cursor || o.MaxNodes != 3 {
				t.Fatal("explicit root options or configured cap changed")
			}
			return domain.RootPage{}, nil
		},
		search: func(_ context.Context, o domain.SearchOptions) (domain.SearchPage, error) {
			if o.Limit != 3 || o.After != searchCursor || o.Query != "cats" {
				t.Fatal("search must preserve cursor and respect configured node cap")
			}
			return domain.SearchPage{}, nil
		},
	}
	s := newService(t, r, 3)
	if _, err := s.ListRoots(t.Context(), domain.RootPageOptions{Limit: 5, Order: domain.RootASC, After: cursor, MaxNodes: 999}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Search(t.Context(), domain.SearchOptions{Limit: 5, Query: " CATS ", After: searchCursor}); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidInputsDoNotReachRepository(t *testing.T) {
	s := newService(t, repositoryStub{}, 100)
	for name, run := range map[string]func() error{
		"blank author": func() error {
			_, err := s.Create(t.Context(), domain.CreateCommentInput{Author: " ", Body: "body"})
			return err
		},
		"blank body": func() error {
			_, err := s.Create(t.Context(), domain.CreateCommentInput{Author: "A", Body: " "})
			return err
		},
		"invalid parent": func() error {
			id := int64(0)
			_, err := s.Create(t.Context(), domain.CreateCommentInput{ParentID: &id, Author: "A", Body: "B"})
			return err
		},
		"delete ID":     func() error { return s.Delete(t.Context(), 0) },
		"subtree ID":    func() error { _, err := s.GetSubtree(t.Context(), -1); return err },
		"root size":     func() error { _, err := s.ListRoots(t.Context(), domain.RootPageOptions{Limit: 51}); return err },
		"negative size": func() error { _, err := s.ListRoots(t.Context(), domain.RootPageOptions{Limit: -1}); return err },
		"root order":    func() error { _, err := s.ListRoots(t.Context(), domain.RootPageOptions{Order: "invalid"}); return err },
		"root cursor": func() error {
			_, err := s.ListRoots(t.Context(), domain.RootPageOptions{After: &domain.RootCursor{ID: 1, CreatedAt: time.Now(), Order: domain.RootASC}})
			return err
		},
		"search size": func() error {
			_, err := s.Search(t.Context(), domain.SearchOptions{Limit: 51, Query: "cats"})
			return err
		},
		"empty query": func() error { _, err := s.Search(t.Context(), domain.SearchOptions{}); return err },
		"search cursor": func() error {
			_, err := s.Search(t.Context(), domain.SearchOptions{Query: "cats", After: &domain.SearchCursor{ID: 1, CreatedAt: time.Now(), Query: "dogs"}})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := run(); !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("expected invalid input, got %v", err)
			}
		})
	}
}

func TestRepositoryErrorsArePreserved(t *testing.T) {
	for _, cause := range []error{domain.ErrInvalidInput, domain.ErrNotFound, domain.ErrParentNotFound, domain.ErrResultTooLarge, domain.ErrConflict, domain.ErrUnavailable, domain.ErrLockUnavailable, domain.ErrQueryInterrupted, domain.ErrInternal, context.Canceled, context.DeadlineExceeded} {
		t.Run(cause.Error(), func(t *testing.T) {
			want := fmt.Errorf("repository: %w", cause)
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			check := func(c context.Context) {
				t.Helper()
				if c != ctx {
					t.Fatal("context identity changed")
				}
			}
			r := repositoryStub{
				create: func(c context.Context, _ domain.CreateCommentInput) (domain.Comment, error) {
					check(c)
					return domain.Comment{}, want
				},
				delete:  func(c context.Context, _ int64) error { check(c); return want },
				subtree: func(c context.Context, _ int64, _ int) ([]domain.Comment, error) { check(c); return nil, want },
				roots: func(c context.Context, _ domain.RootPageOptions) (domain.RootPage, error) {
					check(c)
					return domain.RootPage{}, want
				},
				search: func(c context.Context, _ domain.SearchOptions) (domain.SearchPage, error) {
					check(c)
					return domain.SearchPage{}, want
				},
			}
			s := newService(t, r, 100)
			_, a := s.Create(ctx, domain.CreateCommentInput{Author: "A", Body: "B"})
			b := s.Delete(ctx, 1)
			_, c := s.GetSubtree(ctx, 1)
			_, d := s.ListRoots(ctx, domain.RootPageOptions{})
			_, e := s.Search(ctx, domain.SearchOptions{Query: "cats"})
			for _, got := range []error{a, b, c, d, e} {
				if got != want || !errors.Is(got, cause) {
					t.Fatal("repository error semantics changed")
				}
			}
		})
	}
}
