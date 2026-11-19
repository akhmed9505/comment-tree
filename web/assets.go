// Package web serves the embedded comment interface.
package web

import (
	"embed"
	"net/http"
)

//go:embed index.html styles.css app.js
var assets embed.FS

// Handler serves only the embedded interface files, without exposing directory listings.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var name, contentType string
		switch r.URL.Path {
		case "/":
			name = "index.html"
			contentType = "text/html; charset=utf-8"
		case "/assets/styles.css":
			name = "styles.css"
			contentType = "text/css; charset=utf-8"
		case "/assets/app.js":
			name = "app.js"
			contentType = "text/javascript; charset=utf-8"
		default:
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		data, err := assets.ReadFile(name)
		if err != nil {
			http.Error(w, "Resource unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = w.Write(data)
		}
	})
}
