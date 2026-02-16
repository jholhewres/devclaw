package webui

import (
	"io/fs"
	"net/http"
	"strings"
)

// spaFileServer serves the React SPA from an embedded or provided filesystem.
// For any path that doesn't match a static file, it falls back to index.html
// so that client-side routing (React Router) can handle the path.
type spaFileServer struct {
	root http.FileSystem
}

func newSPAFileServer(fsys fs.FS) *spaFileServer {
	return &spaFileServer{root: http.FS(fsys)}
}

func (s *spaFileServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Never intercept API routes.
	if strings.HasPrefix(path, "/api/") {
		http.NotFound(w, r)
		return
	}

	// Try to serve the exact file (JS, CSS, images, etc.)
	if path != "/" {
		f, err := s.root.Open(path)
		if err == nil {
			f.Close()
			// Static assets (hashed filenames) can be cached aggressively.
			if strings.HasPrefix(path, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			http.FileServer(s.root).ServeHTTP(w, r)
			return
		}
	}

	// For all other paths, serve index.html (SPA fallback).
	// IMPORTANT: no-cache so the browser always gets the latest HTML
	// (which references the latest hashed JS/CSS bundles).
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	r.URL.Path = "/"
	http.FileServer(s.root).ServeHTTP(w, r)
}
