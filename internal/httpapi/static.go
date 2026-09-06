package httpapi

import (
	"io/fs"
	"net/http"
	"strings"
)

// static serves the embedded frontend. Real files are served as they
// are. A navigation request for a path that is not a file gets
// index.html, so the client can read ?link=n and own its own routes. A
// missing asset stays a 404 rather than turning into HTML with a 200.
func static(dist fs.FS) http.Handler {
	files := http.FileServerFS(dist)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.Trim(r.URL.Path, "/")
		switch {
		case name == "":
			serveIndex(w, dist)
		case exists(dist, name):
			if strings.HasPrefix(name, "assets/") {
				// hashed bundle names, safe to cache for as long as allowed
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			files.ServeHTTP(w, r)
		case looksLikeFile(name):
			http.NotFound(w, r)
		default:
			serveIndex(w, dist)
		}
	})
}

func serveIndex(w http.ResponseWriter, dist fs.FS) {
	index, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		http.Error(w, "the frontend is not built into this binary, run make build-web", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// index.html points at the hashed bundles, so it must never go stale
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(index)
}

func exists(dist fs.FS, name string) bool {
	info, err := fs.Stat(dist, name)
	return err == nil && !info.IsDir()
}

// looksLikeFile is true for anything with an extension in its last
// segment: a request like that is for a resource, not a page.
func looksLikeFile(name string) bool {
	last := name[strings.LastIndex(name, "/")+1:]
	return strings.Contains(last, ".")
}
