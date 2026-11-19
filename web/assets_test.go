package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedAssets(t *testing.T) {
	for _, tc := range []struct{ path, kind, content string }{
		{"/", "text/html", "lang=\"ru\""},
		{"/assets/styles.css", "text/css", ".depth-5"},
		{"/assets/app.js", "text/javascript", "document.createElement"},
	} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			w := httptest.NewRecorder()
			Handler().ServeHTTP(w, httptest.NewRequest(method, tc.path, nil))
			if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Type"), tc.kind) {
				t.Fatal("embedded asset not served with its content type")
			}
			if method == http.MethodGet && !strings.Contains(w.Body.String(), tc.content) {
				t.Fatal("embedded content missing")
			}
			if method == http.MethodHead && w.Body.Len() != 0 {
				t.Fatal("HEAD must not return a body")
			}
		}
	}
}

func TestAssetsDoNotExposeOtherFiles(t *testing.T) {
	for _, path := range []string{"/assets/", "/assets/index.html", "/go.mod", "/assets/../assets.go", "/assets/missing.js"} {
		w := httptest.NewRecorder()
		Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusNotFound {
			t.Fatalf("unexpected asset access: %s", path)
		}
	}
	w := httptest.NewRecorder()
	Handler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatal("unsupported asset method accepted")
	}
}
