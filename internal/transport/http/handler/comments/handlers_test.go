package comments

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/akhmed9505/comment-tree/internal/domain"
	svccomments "github.com/akhmed9505/comment-tree/internal/service/comments"
	"github.com/akhmed9505/comment-tree/internal/transport/http/handler/comments/mocks"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/wb-go/wbf/logger"
)

func newTestLogger() *logger.ZerologAdapter {
	return logger.NewZerologAdapter("test", "test")
}

func setupRouter(t *testing.T, mockHandler Service) *gin.Engine {
	t.Helper()

	gin.SetMode(gin.TestMode)
	router := gin.New()

	log := newTestLogger()
	h := New(mockHandler, nil, log)

	router.POST("/comments", h.Create)
	router.DELETE("/comments/:id", h.Delete)
	router.GET("/comments", h.GetRootComments)
	router.GET("/comments/:parent_id/children", h.GetChildComments)
	router.GET("/comments/tree", h.GetCommentTree)

	return router
}

func TestCreate_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHandler := mocks.NewMockCommentHandler(ctrl)
	router := setupRouter(t, mockHandler)

	reqBody := map[string]any{
		"content":   "Test comment",
		"parent_id": 0,
	}
	body, _ := json.Marshal(reqBody)

	mockHandler.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(1, nil)

	req := httptest.NewRequest(http.MethodPost, "/comments", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rec.Code)
	}

	var resp int
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp != 1 {
		t.Fatalf("expected id 1, got %d; body=%s", resp, rec.Body.String())
	}
}

func TestCreate_BadJSON(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHandler := mocks.NewMockCommentHandler(ctrl)
	router := setupRouter(t, mockHandler)

	req := httptest.NewRequest(http.MethodPost, "/comments", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestDelete_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHandler := mocks.NewMockCommentHandler(ctrl)
	router := setupRouter(t, mockHandler)

	id := 123
	mockHandler.EXPECT().
		Delete(gomock.Any(), id).
		Return(nil)

	req := httptest.NewRequest(http.MethodDelete, "/comments/"+strconv.Itoa(id), nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
}

func TestDelete_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHandler := mocks.NewMockCommentHandler(ctrl)
	router := setupRouter(t, mockHandler)

	id := 999
	mockHandler.EXPECT().
		Delete(gomock.Any(), id).
		Return(svccomments.ErrNotFound)

	req := httptest.NewRequest(http.MethodDelete, "/comments/"+strconv.Itoa(id), nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}
}

func TestDelete_InternalError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHandler := mocks.NewMockCommentHandler(ctrl)
	router := setupRouter(t, mockHandler)

	id := 999
	mockHandler.EXPECT().
		Delete(gomock.Any(), id).
		Return(errors.New("db error"))

	req := httptest.NewRequest(http.MethodDelete, "/comments/"+strconv.Itoa(id), nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}
}

func TestGetRootComments_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHandler := mocks.NewMockCommentHandler(ctrl)
	router := setupRouter(t, mockHandler)

	expectedComments := []*domain.Comment{
		{ID: 1, Content: "Comment 1"},
		{ID: 2, Content: "Comment 2"},
	}

	mockHandler.EXPECT().
		GetRootComments(gomock.Any(), gomock.Any(), 2, 0).
		Return(expectedComments, nil)

	req := httptest.NewRequest(http.MethodGet, "/comments?limit=2&offset=0&search=test", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var resp []*domain.Comment
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(resp) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(resp))
	}
}

func TestGetRootComments_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHandler := mocks.NewMockCommentHandler(ctrl)
	router := setupRouter(t, mockHandler)

	mockHandler.EXPECT().
		GetRootComments(gomock.Any(), gomock.Any(), 10, 0).
		Return(nil, errors.New("db error"))

	req := httptest.NewRequest(http.MethodGet, "/comments?limit=10", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}
}

func TestGetChildComments_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHandler := mocks.NewMockCommentHandler(ctrl)
	router := setupRouter(t, mockHandler)

	parentID := 5
	expectedComments := []*domain.Comment{
		{ID: 10, Content: "Child 1"},
	}

	mockHandler.EXPECT().
		GetChildComments(gomock.Any(), parentID).
		Return(expectedComments, nil)

	req := httptest.NewRequest(http.MethodGet, "/comments/5/children", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var resp []*domain.Comment
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(resp) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(resp))
	}
}

func TestGetChildComments_InvalidParentID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHandler := mocks.NewMockCommentHandler(ctrl)
	router := setupRouter(t, mockHandler)

	req := httptest.NewRequest(http.MethodGet, "/comments/abc/children", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestGetChildComments_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHandler := mocks.NewMockCommentHandler(ctrl)
	router := setupRouter(t, mockHandler)

	mockHandler.EXPECT().
		GetChildComments(gomock.Any(), 1).
		Return(nil, errors.New("db error"))

	req := httptest.NewRequest(http.MethodGet, "/comments/1/children", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}
}

func TestGetCommentTree_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHandler := mocks.NewMockCommentHandler(ctrl)
	router := setupRouter(t, mockHandler)

	expectedTree := []domain.CommentNode{
		{
			Comment: domain.Comment{ID: 1, Content: "Root"},
		},
	}

	mockHandler.EXPECT().
		GetAllComments(gomock.Any()).
		Return(expectedTree, nil)

	req := httptest.NewRequest(http.MethodGet, "/comments/tree", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}
