package comments

import (
	"context"
	"errors"
	"testing"

	"github.com/akhmed9505/comment-tree/internal/domain"
	"github.com/akhmed9505/comment-tree/internal/service/comments/mocks"

	"github.com/golang/mock/gomock"
	"github.com/wb-go/wbf/logger"
)

func newTestLogger() *logger.ZerologAdapter {
	return logger.NewZerologAdapter("test", "test")
}

func TestService_Create(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockCommentService(ctrl)
	log := newTestLogger()
	s := New(log, repo)

	comment := &domain.Comment{ID: 1, Content: "Test"}

	repo.EXPECT().
		Create(gomock.Any(), comment).
		Return(1, nil)

	id, err := s.Create(context.Background(), comment)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 1 {
		t.Fatalf("expected id 1, got %d", id)
	}
}

func TestService_Create_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockCommentService(ctrl)
	log := newTestLogger()
	s := New(log, repo)

	comment := &domain.Comment{ID: 1, Content: "Test"}

	repo.EXPECT().
		Create(gomock.Any(), comment).
		Return(0, errors.New("some error"))

	id, err := s.Create(context.Background(), comment)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if id != 0 {
		t.Fatalf("expected id 0, got %d", id)
	}
}

func TestService_Delete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockCommentService(ctrl)
	log := newTestLogger()
	s := New(log, repo)

	repo.EXPECT().
		Delete(gomock.Any(), 42).
		Return(nil)

	if err := s.Delete(context.Background(), 42); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestService_Delete_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockCommentService(ctrl)
	log := newTestLogger()
	s := New(log, repo)

	repo.EXPECT().
		Delete(gomock.Any(), 42).
		Return(errors.New("delete error"))

	err := s.Delete(context.Background(), 42)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "delete error" {
		t.Fatalf("expected 'delete error', got %v", err)
	}
}

func TestService_GetRootComments(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockCommentService(ctrl)
	log := newTestLogger()
	s := New(log, repo)

	search := "test"
	expected := []*domain.Comment{
		{ID: 1, Content: "Comment 1"},
		{ID: 2, Content: "Comment 2"},
	}

	repo.EXPECT().
		GetRootComments(gomock.Any(), &search, 10, 0).
		Return(expected, nil)

	result, err := s.GetRootComments(context.Background(), &search, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(result))
	}
}

func TestService_GetRootComments_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockCommentService(ctrl)
	log := newTestLogger()
	s := New(log, repo)

	search := "test"

	repo.EXPECT().
		GetRootComments(gomock.Any(), &search, 10, 0).
		Return(nil, errors.New("db error"))

	_, err := s.GetRootComments(context.Background(), &search, 10, 0)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestService_GetChildComments(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockCommentService(ctrl)
	log := newTestLogger()
	s := New(log, repo)

	expected := []*domain.Comment{
		{ID: 10, Content: "Child comment"},
	}

	repo.EXPECT().
		GetChildComments(gomock.Any(), 5).
		Return(expected, nil)

	result, err := s.GetChildComments(context.Background(), 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(result))
	}
}

func TestService_GetChildComments_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockCommentService(ctrl)
	log := newTestLogger()
	s := New(log, repo)

	repo.EXPECT().
		GetChildComments(gomock.Any(), 5).
		Return(nil, errors.New("db error"))

	_, err := s.GetChildComments(context.Background(), 5)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestService_GetAllComments(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockCommentService(ctrl)
	log := newTestLogger()
	s := New(log, repo)

	expected := []domain.CommentNode{
		{
			Comment: domain.Comment{ID: 1, Content: "Root"},
		},
	}

	repo.EXPECT().
		GetAllComments(gomock.Any()).
		Return(expected, nil)

	result, err := s.GetAllComments(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 node, got %d", len(result))
	}
}

func TestService_GetAllComments_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockCommentService(ctrl)
	log := newTestLogger()
	s := New(log, repo)

	repo.EXPECT().
		GetAllComments(gomock.Any()).
		Return(nil, errors.New("db error"))

	_, err := s.GetAllComments(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
