// Package web embeds the browser console and serves it from the API origin, so
// the demo needs no separate frontend build or dev server.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:static
var assets embed.FS

// FS returns the console files rooted at the static directory.
func FS() fs.FS {
	sub, err := fs.Sub(assets, "static")
	if err != nil {
		panic(err)
	}
	return sub
}

// Handler serves the single-page console. Paths that do not exist on disk fall
// back to index.html so refreshing a deep link keeps working; unknown /api paths
// are delegated to notFound, which keeps the JSON error envelope.
func Handler(notFound http.Handler) http.Handler {
	root := FS()
	files := http.FileServer(http.FS(root))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			notFound.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")

		clean := path.Clean(r.URL.Path)
		if clean != "/" {
			if f, err := root.Open(strings.TrimPrefix(clean, "/")); err == nil {
				_ = f.Close()
				w.Header().Set("Cache-Control", "no-cache")
				files.ServeHTTP(w, r)
				return
			}
		}
		// The console is a single document: unknown routes render it and the
		// client decides what to show.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFileFS(w, r, root, "index.html")
	})
}
