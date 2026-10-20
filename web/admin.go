package web

import (
	"net/http"
	"path"
	"strings"
)

// AdminHandler serves the operations console from /admin/.
//
// It is a separate handler from Handler because the customer console is a single
// document at "/" while the admin console lives under a sub-path and must not
// swallow unknown /admin/* routes into the customer page.
func AdminHandler(notFound http.Handler) http.Handler {
	root := FS()
	files := http.StripPrefix("/admin/", http.FileServer(http.FS(root)))

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
		// A missing trailing slash would make relative asset URLs resolve against
		// "/" instead of "/admin/", so redirect once, before anything else.
		if r.URL.Path == "/admin" {
			http.Redirect(w, r, "/admin/", http.StatusMovedPermanently)
			return
		}

		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-cache")

		rel := strings.TrimPrefix(path.Clean(r.URL.Path), "/admin/")
		if rel != "" && rel != "." {
			if f, err := root.Open(rel); err == nil {
				_ = f.Close()
				files.ServeHTTP(w, r)
				return
			}
		}
		// The console is a hash-routed single document.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeFileFS(w, r, root, "admin.html")
	})
}
